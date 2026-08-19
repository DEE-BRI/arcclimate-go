package arcclimate

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/klauspost/compress/zstd"
)

// このファイルは、MSM Parquetファイルを読むために必要な最小限の機能のみを持つ
// 自前のParquetリーダーです。外部のParquet/Thriftライブラリには依存せず、
// 圧縮解凍のみ klauspost/compress/zstd (単体で追加の依存を持たない) を使用します。
//
// 対応範囲は実データ(msm_2010_2025/*.parquet 書き込み元: github.com/parquet-go/parquet-go、
// および msm_2011_2025/*.parquet 書き込み元: pyarrow)で確認済みの以下の特性に限定しています。
// 想定外のファイルに遭遇した場合は黙って誤ったデータを返すのではなく、
// 必ずpanicで検出できるようにしています。
//   - フラットなスキーマ(ネストなし)
//   - 全列がrequired(non-nullable) → definition/repetition levelは常に0バイト
//   - encodingはPLAIN、またはBYTE_STREAM_SPLIT(float列)、DELTA_BINARY_PACKED(int64列)
//     (辞書エンコードなし)
//   - 圧縮コーデックはUNCOMPRESSEDまたはZSTD
//   - ページ形式はDATA_PAGE(V1)またはDATA_PAGE_V2

// ---- Thrift Compact Protocol (読込専用の最小実装) ----

const (
	thriftTypeBoolTrue  = 1
	thriftTypeBoolFalse = 2
	thriftTypeByte      = 3
	thriftTypeI16       = 4
	thriftTypeI32       = 5
	thriftTypeI64       = 6
	thriftTypeDouble    = 7
	thriftTypeBinary    = 8
	thriftTypeList      = 9
	thriftTypeSet       = 10
	thriftTypeStruct    = 12
)

type thriftReader struct {
	data []byte
	pos  int64
}

func (r *thriftReader) readByte() byte {
	b := r.data[r.pos]
	r.pos++
	return b
}

func (r *thriftReader) readVarint() uint64 {
	var result uint64
	var shift uint
	for {
		b := r.readByte()
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
	}
	return result
}

func (r *thriftReader) readZigzag() int64 {
	v := r.readVarint()
	return int64(v>>1) ^ -int64(v&1)
}

func (r *thriftReader) readBinary() []byte {
	n := int64(r.readVarint())
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *thriftReader) readListHeader() (elemType int, size int) {
	lh := r.readByte()
	size = int(lh >> 4)
	elemType = int(lh & 0x0f)
	if size == 15 {
		size = int(r.readVarint())
	}
	return elemType, size
}

// readThriftStruct は構造体のフィールドを順に読み、フィールドごとにhandlerを呼び出します。
// handlerは呼ばれたら必ず対応する値を読み進める(またはskipThriftValueで読み飛ばす)責任を持ちます。
func readThriftStruct(r *thriftReader, handler func(fieldID, thriftType int)) {
	lastID := 0
	for {
		header := r.readByte()
		if header == 0 {
			return // STOP
		}
		delta := int(header >> 4)
		ftype := int(header & 0x0f)
		var fieldID int
		if delta == 0 {
			fieldID = int(r.readZigzag())
		} else {
			fieldID = lastID + delta
		}
		lastID = fieldID
		handler(fieldID, ftype)
	}
}

// skipThriftValue は不要なフィールドの値を読み飛ばします。
// 未知の型は、想定外のParquetファイル構造を握りつぶさないためpanicします。
func skipThriftValue(r *thriftReader, ftype int) {
	switch ftype {
	case thriftTypeBoolTrue, thriftTypeBoolFalse:
		// 値は型ニブル自体に埋め込まれており、追加のバイトは無い
	case thriftTypeByte:
		r.readByte()
	case thriftTypeI16, thriftTypeI32, thriftTypeI64:
		r.readZigzag()
	case thriftTypeDouble:
		r.pos += 8
	case thriftTypeBinary:
		r.readBinary()
	case thriftTypeList, thriftTypeSet:
		elemType, size := r.readListHeader()
		for i := 0; i < size; i++ {
			skipThriftValue(r, elemType)
		}
	case thriftTypeStruct:
		readThriftStruct(r, func(fieldID, ftype int) {
			skipThriftValue(r, ftype)
		})
	default:
		panic(fmt.Sprintf("parquet: 未対応のthrift型です(想定外のParquetファイル構造): %d", ftype))
	}
}

// ---- Parquetフォーマット定数 ----

const (
	parquetPhysicalTypeInt64 = 2
	parquetPhysicalTypeFloat = 4

	parquetEncodingPlain             = 0
	parquetEncodingDeltaBinaryPacked = 5
	parquetEncodingByteStreamSplit   = 9

	parquetCodecUncompressed = 0
	parquetCodecZstd         = 6

	parquetPageTypeDataPage   = 0
	parquetPageTypeDataPageV2 = 3
)

// parquetColumnMeta は1つのColumnChunkから読み取った、値の読込に必要な最小限のメタデータ
type parquetColumnMeta struct {
	Name                string
	PhysicalType        int64
	Codec               int64
	NumValues           int64
	DataPageOffset      int64
	TotalCompressedSize int64
	Optional            bool // スキーマ上OPTIONAL(nullable)かどうか(SchemaElement.repetition_typeより)
}

const (
	parquetFieldRepetitionRequired = 0
	parquetFieldRepetitionOptional = 1
	parquetFieldRepetitionRepeated = 2
)

// parquetFile はメモリ上に読み込んだParquetファイルの内容と、解析済みの列メタデータを保持します。
type parquetFile struct {
	data          []byte
	numRows       int64
	columnsByName map[string][]parquetColumnMeta
}

// openParquetFile はParquetファイルのフッター(FileMetaData)を解析します。
// dataにはファイル全体のバイト列を渡します(このパッケージが扱うMSMファイルは
// 数MB程度と小さいため、全体をメモリに読み込む前提にしています)。
func openParquetFile(data []byte) *parquetFile {
	numRows, columnsByName := readParquetFooter(data)
	return &parquetFile{data: data, numRows: numRows, columnsByName: columnsByName}
}

// readParquetFooter はファイル末尾のFileMetaDataを読み、全RowGroup分の列メタデータを
// 列名をキーにしたマップとして返します。複数RowGroupにまたがる同名列は出現順に連結されます。
func readParquetFooter(data []byte) (numRows int64, columnsByName map[string][]parquetColumnMeta) {
	if len(data) < 12 || string(data[len(data)-4:]) != "PAR1" {
		panic("parquet: PAR1マジックバイトが見つかりません(Parquetファイルとして不正です)")
	}
	footerLen := binary.LittleEndian.Uint32(data[len(data)-8 : len(data)-4])
	footerStart := int64(len(data)) - 8 - int64(footerLen)
	if footerStart < 4 {
		panic("parquet: フッター長が不正です")
	}

	r := &thriftReader{data: data, pos: footerStart}
	columnsByName = map[string][]parquetColumnMeta{}
	var optionalByName map[string]bool

	readThriftStruct(r, func(fieldID, ftype int) {
		switch fieldID {
		case 2: // schema (list<SchemaElement>)
			optionalByName = readSchemaOptionalByName(r)
		case 3: // num_rows
			numRows = r.readZigzag()
		case 4: // row_groups (list<RowGroup>)
			elemType, size := r.readListHeader()
			if elemType != thriftTypeStruct {
				panic("parquet: row_groupsの要素型が不正です")
			}
			for i := 0; i < size; i++ {
				readThriftStruct(r, func(fieldID, ftype int) {
					if fieldID == 1 { // columns (list<ColumnChunk>)
						colElemType, colSize := r.readListHeader()
						if colElemType != thriftTypeStruct {
							panic("parquet: columnsの要素型が不正です")
						}
						for j := 0; j < colSize; j++ {
							meta := readColumnChunk(r)
							columnsByName[meta.Name] = append(columnsByName[meta.Name], meta)
						}
					} else {
						skipThriftValue(r, ftype)
					}
				})
			}
		default:
			skipThriftValue(r, ftype)
		}
	})

	for name, metas := range columnsByName {
		optional := optionalByName[name]
		for i := range metas {
			metas[i].Optional = optional
		}
	}

	return numRows, columnsByName
}

// readSchemaOptionalByName はFileMetaData.schema(list<SchemaElement>、ルート要素を含む
// フラットなpre-order列)を読み、列名(ルート以外)がOPTIONAL(nullable)かどうかのマップを返します。
// REPEATEDは非対応のためpanicします(このパッケージが扱うMSMデータに現れない構造のため)。
func readSchemaOptionalByName(r *thriftReader) map[string]bool {
	elemType, size := r.readListHeader()
	if elemType != thriftTypeStruct {
		panic("parquet: schemaの要素型が不正です")
	}

	optionalByName := map[string]bool{}
	for i := 0; i < size; i++ {
		var name string
		var hasRepetitionType bool
		var repetitionType int64
		readThriftStruct(r, func(fieldID, ftype int) {
			switch fieldID {
			case 3: // repetition_type
				repetitionType = r.readZigzag()
				hasRepetitionType = true
			case 4: // name
				name = string(r.readBinary())
			default:
				skipThriftValue(r, ftype)
			}
		})
		if !hasRepetitionType {
			continue // ルート要素(repetition_typeを持たない)はスキップ
		}
		switch repetitionType {
		case parquetFieldRepetitionRequired:
			optionalByName[name] = false
		case parquetFieldRepetitionOptional:
			optionalByName[name] = true
		default:
			panic(fmt.Sprintf("parquet: 未対応のrepetition_typeです(REQUIRED/OPTIONALのみ対応): %s=%d", name, repetitionType))
		}
	}
	return optionalByName
}

// readColumnChunk はColumnChunk構造体から必要なメタデータのみ抽出します。
func readColumnChunk(r *thriftReader) parquetColumnMeta {
	var meta parquetColumnMeta
	readThriftStruct(r, func(fieldID, ftype int) {
		if fieldID != 3 { // meta_data (ColumnMetaData) 以外は不要
			skipThriftValue(r, ftype)
			return
		}
		readThriftStruct(r, func(fieldID, ftype int) {
			switch fieldID {
			case 1: // type
				meta.PhysicalType = r.readZigzag()
			case 3: // path_in_schema (list<string>)
				elemType, size := r.readListHeader()
				if elemType != thriftTypeBinary {
					panic("parquet: path_in_schemaの要素型が不正です")
				}
				var path []string
				for i := 0; i < size; i++ {
					path = append(path, string(r.readBinary()))
				}
				if len(path) != 1 {
					panic(fmt.Sprintf("parquet: ネストしたスキーマは非対応です(path_in_schema=%v)", path))
				}
				meta.Name = path[0]
			case 4: // codec
				meta.Codec = r.readZigzag()
			case 5: // num_values
				meta.NumValues = r.readZigzag()
			case 7: // total_compressed_size
				meta.TotalCompressedSize = r.readZigzag()
			case 9: // data_page_offset
				meta.DataPageOffset = r.readZigzag()
			default:
				skipThriftValue(r, ftype)
			}
		})
	})
	return meta
}

// ---- ページ読込 ----

// dataPageHeader はDATA_PAGE(V1)またはDATA_PAGE_V2のPageHeaderから読み取った、
// 値の取り出しに必要な情報(両バージョンを共通の形で扱う)
type dataPageHeader struct {
	PageType             int64
	CompressedPageSize   int64
	UncompressedPageSize int64
	NumValues            int64
	Encoding             int64
	DefLevelsByteLength  int64 // V2のみ。V1は常に0(このパッケージが対応するrequired列のみのため)
	RepLevelsByteLength  int64 // V2のみ。V1は常に0(同上)
	IsCompressed         bool
}

// readPageHeader はPageHeaderを読みます。DATA_PAGE(V1)とDATA_PAGE_V2に対応します。
// definition/repetition levelが存在する場合はpanicします(このパッケージが対応する
// 実データの特性(フラットなrequired列)から外れるため)。
func readPageHeader(r *thriftReader) dataPageHeader {
	var ph dataPageHeader
	ph.IsCompressed = true // Thrift仕様上のデフォルト値(フィールド省略時。V1にはこのフィールド自体が無く常に圧縮されている)

	readThriftStruct(r, func(fieldID, ftype int) {
		switch fieldID {
		case 1:
			ph.PageType = r.readZigzag()
		case 2:
			ph.UncompressedPageSize = r.readZigzag()
		case 3:
			ph.CompressedPageSize = r.readZigzag()
		case 5: // data_page_header (V1)
			readThriftStruct(r, func(fieldID, ftype int) {
				switch fieldID {
				case 1:
					ph.NumValues = r.readZigzag()
				case 2:
					ph.Encoding = r.readZigzag()
				default:
					skipThriftValue(r, ftype)
				}
			})
		case 8: // data_page_header_v2
			readThriftStruct(r, func(fieldID, ftype int) {
				switch fieldID {
				case 1:
					ph.NumValues = r.readZigzag()
				case 4:
					ph.Encoding = r.readZigzag()
				case 5:
					ph.DefLevelsByteLength = r.readZigzag()
				case 6:
					ph.RepLevelsByteLength = r.readZigzag()
				case 7:
					ph.IsCompressed = ftype == thriftTypeBoolTrue
				default:
					skipThriftValue(r, ftype)
				}
			})
		default:
			skipThriftValue(r, ftype)
		}
	})

	if ph.PageType != parquetPageTypeDataPage && ph.PageType != parquetPageTypeDataPageV2 {
		panic(fmt.Sprintf("parquet: 未対応のページ種別です(DATA_PAGE/DATA_PAGE_V2のみ対応): %d", ph.PageType))
	}
	switch ph.Encoding {
	case parquetEncodingPlain, parquetEncodingByteStreamSplit, parquetEncodingDeltaBinaryPacked:
	default:
		panic(fmt.Sprintf("parquet: 未対応のencodingです(PLAIN/BYTE_STREAM_SPLIT/DELTA_BINARY_PACKEDのみ対応): %d", ph.Encoding))
	}
	if ph.RepLevelsByteLength != 0 {
		panic("parquet: repeatedな列は非対応です(repetition levelを検出しました)")
	}

	return ph
}

// readColumnValueBytes は1つの列の全ページを読み、値をPLAIN encoding相当の生バイト列
// (固定長: float32なら4byte LE、int64なら8byte LE)に変換して連結し返します
// (必要であれば複数RowGroup分のmetasを順に処理します)。全行数(null含む)分を返し、
// meta.Optionalな列でnullの要素はfloat32ならNaNで埋めます(int64列でのnullは非対応でpanicします)。
// ページのencodingがPLAIN以外(BYTE_STREAM_SPLIT/DELTA_BINARY_PACKED)の場合は、
// ここでPLAIN相当の形へデコードします。
func readColumnValueBytes(data []byte, metas []parquetColumnMeta) []byte {
	var out []byte
	var dec *zstd.Decoder

	for _, meta := range metas {
		if meta.Codec != parquetCodecUncompressed && meta.Codec != parquetCodecZstd {
			panic(fmt.Sprintf("parquet: 未対応の圧縮コーデックです(UNCOMPRESSED/ZSTDのみ対応): %d", meta.Codec))
		}
		if meta.Codec == parquetCodecZstd && dec == nil {
			var err error
			dec, err = zstd.NewReader(nil)
			if err != nil {
				panic(err)
			}
			defer dec.Close()
		}

		pos := meta.DataPageOffset
		end := meta.DataPageOffset + meta.TotalCompressedSize
		var valuesRead int64

		for pos < end && valuesRead < meta.NumValues {
			r := &thriftReader{data: data, pos: pos}
			ph := readPageHeader(r)
			pageDataStart := r.pos

			var defLevels []int64
			var valueBytes []byte

			switch ph.PageType {
			case parquetPageTypeDataPage: // V1: ページ全体(def levelも含む)が丸ごと圧縮されている
				compressed := data[pageDataStart : pageDataStart+ph.CompressedPageSize]
				var pageRaw []byte
				// V1のPageHeaderにはis_compressedフィールドが無くph.IsCompressedは常にtrue固定
				// (デフォルト値)なので、実際に解凍が必要かはcodecで判定する
				// (UNCOMPRESSEDならdecが未初期化のまま呼ぶとpanicするため)
				if meta.Codec != parquetCodecUncompressed {
					var err error
					pageRaw, err = dec.DecodeAll(compressed, make([]byte, 0, ph.UncompressedPageSize))
					if err != nil {
						panic(err)
					}
				} else {
					pageRaw = compressed
				}
				if meta.Optional {
					defLevels, valueBytes = extractV1DefLevels(pageRaw, ph.NumValues)
				} else {
					valueBytes = pageRaw
				}

			case parquetPageTypeDataPageV2: // V2: def/rep levelは非圧縮、値データのみ圧縮対象
				if meta.Optional != (ph.DefLevelsByteLength > 0) {
					panic("parquet: スキーマのoptional有無とページのdefinition levelの有無が一致しません")
				}
				defRepLen := ph.DefLevelsByteLength + ph.RepLevelsByteLength
				if defRepLen > 0 {
					defLevels = decodeDefLevels(data[pageDataStart:pageDataStart+ph.DefLevelsByteLength], ph.NumValues)
				}
				valuesCompressed := data[pageDataStart+defRepLen : pageDataStart+ph.CompressedPageSize]
				// V2はis_compressedフラグを持つが、念のためcodecとも矛盾がないか確認する
				// (UNCOMPRESSEDなのにis_compressed=trueという不整合なファイルに遭遇した場合、
				// decが未初期化のままdec.DecodeAllを呼びpanicするのを避けるため)
				if ph.IsCompressed && meta.Codec != parquetCodecUncompressed {
					var err error
					valueBytes, err = dec.DecodeAll(valuesCompressed, make([]byte, 0, ph.UncompressedPageSize-defRepLen))
					if err != nil {
						panic(err)
					}
				} else {
					valueBytes = valuesCompressed
				}
			}

			numNonNull := ph.NumValues
			if meta.Optional {
				numNonNull = countNonNull(defLevels)
			}

			decoded := decodePageValues(valueBytes, ph.Encoding, meta.PhysicalType, numNonNull)
			if meta.Optional {
				decoded = expandWithNulls(decoded, defLevels, meta.PhysicalType)
			}
			out = append(out, decoded...)

			valuesRead += ph.NumValues
			pos = pageDataStart + ph.CompressedPageSize
		}
	}

	return out
}

// physicalTypeSize は物理型のPLAIN encoding時の固定バイト長を返します。
func physicalTypeSize(physicalType int64) int {
	switch physicalType {
	case parquetPhysicalTypeFloat:
		return 4
	case parquetPhysicalTypeInt64:
		return 8
	default:
		panic(fmt.Sprintf("parquet: 未対応の物理型です: %d", physicalType))
	}
}

// decodeDefLevels はRLE/Bit-Packed Hybrid形式(bit-width=1固定)でエンコードされた
// definition levelをデコードします。このパッケージが対応するのは
// max definition level=1(nullable・non-repeatedな列)のみです。
func decodeDefLevels(data []byte, numValues int64) []int64 {
	levels := make([]int64, 0, numValues)
	tr := &thriftReader{data: data, pos: 0}
	for int64(len(levels)) < numValues {
		header := tr.readVarint()
		if header&1 == 0 {
			runLen := int64(header >> 1)
			value := int64(tr.readByte()) // bit-width=1 -> byte-width=1
			for i := int64(0); i < runLen; i++ {
				levels = append(levels, value)
			}
		} else {
			groupCount := int64(header >> 1)
			for g := int64(0); g < groupCount; g++ {
				b := tr.readByte()
				for bit := 0; bit < 8; bit++ {
					if int64(len(levels)) < numValues {
						levels = append(levels, int64((b>>uint(bit))&1))
					}
				}
			}
		}
	}
	return levels
}

// extractV1DefLevels はDATA_PAGE(V1)の展開済みデータ先頭にある
// [4byte LE length][RLE/Bit-Packed Hybrid data] 形式のdefinition levelを取り出し、
// 残り(値データ)と共に返します。
func extractV1DefLevels(pageRaw []byte, numValues int64) (defLevels []int64, rest []byte) {
	if len(pageRaw) < 4 {
		panic("parquet: V1ページのdefinition level長プレフィックスが読めません")
	}
	length := int64(binary.LittleEndian.Uint32(pageRaw[0:4]))
	defLevels = decodeDefLevels(pageRaw[4:4+length], numValues)
	return defLevels, pageRaw[4+length:]
}

// countNonNull はdefinition levelのうち非null(1)の件数を数えます。
func countNonNull(levels []int64) int64 {
	var n int64
	for _, l := range levels {
		if l != 0 {
			n++
		}
	}
	return n
}

// expandWithNulls は非null要素のみを詰めたPLAIN相当バイト列を、definition levelに基づいて
// 全要素数分に展開します。null位置は、float32ならNaNビットパターンで埋めます
// (int64列でnullが実際に発生した場合は想定外としてpanicします)。
func expandWithNulls(nonNullBytes []byte, defLevels []int64, physicalType int64) []byte {
	elemSize := physicalTypeSize(physicalType)
	out := make([]byte, len(defLevels)*elemSize)
	srcPos := 0
	for i, level := range defLevels {
		if level != 0 {
			copy(out[i*elemSize:(i+1)*elemSize], nonNullBytes[srcPos:srcPos+elemSize])
			srcPos += elemSize
			continue
		}
		switch physicalType {
		case parquetPhysicalTypeFloat:
			binary.LittleEndian.PutUint32(out[i*elemSize:(i+1)*elemSize], math.Float32bits(float32(math.NaN())))
		default:
			panic(fmt.Sprintf("parquet: この型でのnull値は非対応です(type=%d)", physicalType))
		}
	}
	if srcPos != len(nonNullBytes) {
		panic("parquet: definition levelの非null数と値データの長さが一致しません")
	}
	return out
}

// decodePageValues は1ページ分の展開済みデータをPLAIN encoding相当の固定長バイト列に変換します。
func decodePageValues(raw []byte, encoding int64, physicalType int64, numValues int64) []byte {
	switch encoding {
	case parquetEncodingPlain:
		return raw
	case parquetEncodingByteStreamSplit:
		if physicalType != parquetPhysicalTypeFloat {
			panic(fmt.Sprintf("parquet: BYTE_STREAM_SPLITはFLOAT型のみ対応です(type=%d)", physicalType))
		}
		return decodeByteStreamSplitFloat32(raw, numValues)
	case parquetEncodingDeltaBinaryPacked:
		if physicalType != parquetPhysicalTypeInt64 {
			panic(fmt.Sprintf("parquet: DELTA_BINARY_PACKEDはINT64型のみ対応です(type=%d)", physicalType))
		}
		values := decodeDeltaBinaryPackedInt64(raw, numValues)
		out := make([]byte, len(values)*8)
		for i, v := range values {
			binary.LittleEndian.PutUint64(out[i*8:i*8+8], uint64(v))
		}
		return out
	default:
		panic(fmt.Sprintf("parquet: 未対応のencodingです: %d", encoding))
	}
}

// decodeByteStreamSplitFloat32 はBYTE_STREAM_SPLITでエンコードされたfloat32値をPLAIN相当
// (4byte LEの連続)にデコードします。BYTE_STREAM_SPLITは、各値のバイトをバイト位置ごとに
// 束ねて4本のストリームとして格納する形式です(stream[b][i] = value[i]のbyte b)。
func decodeByteStreamSplitFloat32(raw []byte, numValues int64) []byte {
	const width = 4
	n := int(numValues)
	if int64(len(raw)) != numValues*width {
		panic(fmt.Sprintf("parquet: BYTE_STREAM_SPLITのデータ長が不正です: got=%d want=%d", len(raw), numValues*width))
	}

	out := make([]byte, len(raw))
	for i := 0; i < n; i++ {
		for b := 0; b < width; b++ {
			out[i*width+b] = raw[b*n+i]
		}
	}
	return out
}

// bitUnpacker はLSB-firstでbit-packされたバイト列から固定幅の値を順に取り出します。
type bitUnpacker struct {
	data    []byte
	bytePos int
	bitPos  uint // 現在のバイト内で次に読むビット位置(0=LSB)
}

func (b *bitUnpacker) readBits(width int) uint64 {
	if width == 0 {
		return 0
	}
	var result uint64
	var resultBits uint
	for resultBits < uint(width) {
		if b.bytePos >= len(b.data) {
			panic("parquet: bit-packedデータの読込中にデータが不足しました")
		}
		avail := 8 - b.bitPos
		need := uint(width) - resultBits
		take := avail
		if take > need {
			take = need
		}
		chunk := (uint64(b.data[b.bytePos]) >> b.bitPos) & ((uint64(1) << take) - 1)
		result |= chunk << resultBits
		resultBits += take
		b.bitPos += take
		if b.bitPos == 8 {
			b.bitPos = 0
			b.bytePos++
		}
	}
	return result
}

// alignToByte は現在位置を次のバイト境界へ進めます(bit-packブロックの後、
// 次のvarintフィールドを読む前に呼びます)。
func (b *bitUnpacker) alignToByte() {
	if b.bitPos != 0 {
		b.bitPos = 0
		b.bytePos++
	}
}

// decodeDeltaBinaryPackedInt64 はDELTA_BINARY_PACKEDでエンコードされたint64値をデコードします。
// フォーマット(Parquet仕様): <block size><miniblocks per block><total value count><first value>
// に続けてブロックが並ぶ。各ブロックは <min delta><miniblock毎のbit width(1byte)>
// <miniblock毎のbit-packedデータ> という構成。
func decodeDeltaBinaryPackedInt64(raw []byte, numValues int64) []int64 {
	// ヘッダー部分はバイト境界のvarint(ULEB128)/zigzag varintなので thriftReader を流用できる
	hr := &thriftReader{data: raw, pos: 0}
	blockSizeInValues := int64(hr.readVarint())
	numMiniblocksInBlock := int64(hr.readVarint())
	totalValueCount := int64(hr.readVarint())
	firstValue := hr.readZigzag()

	if totalValueCount != numValues {
		panic(fmt.Sprintf("parquet: DELTA_BINARY_PACKEDのtotal_value_countが不一致です: header=%d page=%d", totalValueCount, numValues))
	}
	if numMiniblocksInBlock <= 0 || blockSizeInValues%numMiniblocksInBlock != 0 {
		panic("parquet: DELTA_BINARY_PACKEDのブロック構成が不正です")
	}
	miniblockSizeInValues := blockSizeInValues / numMiniblocksInBlock

	values := make([]int64, 0, totalValueCount)
	values = append(values, firstValue)

	bu := &bitUnpacker{data: raw, bytePos: int(hr.pos)}
	prev := firstValue

	for int64(len(values)) < totalValueCount {
		minDelta := readZigzagFromBits(bu)
		bitWidths := make([]int, numMiniblocksInBlock)
		for i := range bitWidths {
			bitWidths[i] = int(bu.readBits(8))
		}

		for mb := int64(0); mb < numMiniblocksInBlock; mb++ {
			width := bitWidths[mb]
			for i := int64(0); i < miniblockSizeInValues; i++ {
				bits := bu.readBits(width)
				if int64(len(values)) < totalValueCount {
					delta := int64(bits) + minDelta
					prev += delta
					values = append(values, prev)
				}
			}
		}
	}

	return values
}

// readZigzagFromBits はバイト境界にあるzigzag varintをbitUnpacker経由で読みます
// (min_deltaの読込用。値そのものはビットパック対象ではなくバイト単位のvarint)。
func readZigzagFromBits(b *bitUnpacker) int64 {
	if b.bitPos != 0 {
		panic("parquet: DELTA_BINARY_PACKEDのブロック境界がバイト境界にありません")
	}
	tr := &thriftReader{data: b.data, pos: int64(b.bytePos)}
	v := tr.readZigzag()
	b.bytePos = int(tr.pos)
	return v
}

// readFloatColumn は名前を指定してfloat32列を読み出します(PLAIN encoding, 4byte LE)。
func (f *parquetFile) readFloatColumn(name string) []float32 {
	metas, ok := f.columnsByName[name]
	if !ok {
		panic(fmt.Sprintf("parquet: 列が見つかりません: %s", name))
	}
	for _, m := range metas {
		if m.PhysicalType != parquetPhysicalTypeFloat {
			panic(fmt.Sprintf("parquet: 列%sの型がFLOATではありません(type=%d)", name, m.PhysicalType))
		}
	}

	raw := readColumnValueBytes(f.data, metas)
	n := len(raw) / 4
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := binary.LittleEndian.Uint32(raw[i*4 : i*4+4])
		out[i] = math.Float32frombits(bits)
	}
	return out
}

// readInt64Column は名前を指定してint64列を読み出します(PLAIN encoding, 8byte LE)。
func (f *parquetFile) readInt64Column(name string) []int64 {
	metas, ok := f.columnsByName[name]
	if !ok {
		panic(fmt.Sprintf("parquet: 列が見つかりません: %s", name))
	}
	for _, m := range metas {
		if m.PhysicalType != parquetPhysicalTypeInt64 {
			panic(fmt.Sprintf("parquet: 列%sの型がINT64ではありません(type=%d)", name, m.PhysicalType))
		}
	}

	raw := readColumnValueBytes(f.data, metas)
	n := len(raw) / 8
	out := make([]int64, n)
	for i := 0; i < n; i++ {
		out[i] = int64(binary.LittleEndian.Uint64(raw[i*8 : i*8+8]))
	}
	return out
}
