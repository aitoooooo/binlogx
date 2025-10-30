package util

// DataType 代表 MySQL 数据类型的分类
type DataType string

const (
	// 数值类型
	TypeTinyInt    DataType = "TINYINT"
	TypeSmallInt   DataType = "SMALLINT"
	TypeMediumInt  DataType = "MEDIUMINT"
	TypeInt        DataType = "INT"
	TypeBigInt     DataType = "BIGINT"
	TypeDecimal    DataType = "DECIMAL"
	TypeNumeric    DataType = "NUMERIC"
	TypeFloat      DataType = "FLOAT"
	TypeDouble     DataType = "DOUBLE"
	TypeBit        DataType = "BIT"

	// 字符串类型
	TypeChar       DataType = "CHAR"
	TypeVarchar    DataType = "VARCHAR"
	TypeBinary     DataType = "BINARY"
	TypeVarbinary  DataType = "VARBINARY"
	TypeText       DataType = "TEXT"
	TypeBlob       DataType = "BLOB"
	TypeEnum       DataType = "ENUM"
	TypeSet        DataType = "SET"

	// 时间类型
	TypeDate       DataType = "DATE"
	TypeTime       DataType = "TIME"
	TypeDatetime   DataType = "DATETIME"
	TypeTimestamp  DataType = "TIMESTAMP"
	TypeYear       DataType = "YEAR"

	// JSON 类型
	TypeJSON       DataType = "JSON"

	// 几何类型
	TypeGeometry   DataType = "GEOMETRY"
	TypePoint      DataType = "POINT"
	TypeLinestring DataType = "LINESTRING"
	TypePolygon    DataType = "POLYGON"
)

// IsNumericType 检查是否为数值类型
func IsNumericType(dt DataType) bool {
	switch dt {
	case TypeTinyInt, TypeSmallInt, TypeMediumInt, TypeInt, TypeBigInt,
		TypeDecimal, TypeNumeric, TypeFloat, TypeDouble, TypeBit:
		return true
	}
	return false
}

// IsStringType 检查是否为字符串类型
func IsStringType(dt DataType) bool {
	switch dt {
	case TypeChar, TypeVarchar, TypeText, TypeEnum, TypeSet:
		return true
	}
	return false
}

// IsBinaryType 检查是否为二进制类型
func IsBinaryType(dt DataType) bool {
	switch dt {
	case TypeBinary, TypeVarbinary, TypeBlob:
		return true
	}
	return false
}

// IsDateTimeType 检查是否为日期时间类型
func IsDateTimeType(dt DataType) bool {
	switch dt {
	case TypeDate, TypeTime, TypeDatetime, TypeTimestamp, TypeYear:
		return true
	}
	return false
}

// IsUnsignedType 检查类型是否支持 unsigned
func IsUnsignedType(dt DataType) bool {
	switch dt {
	case TypeTinyInt, TypeSmallInt, TypeMediumInt, TypeInt, TypeBigInt,
		TypeDecimal, TypeNumeric, TypeFloat, TypeDouble:
		return true
	}
	return false
}

// NeedsQuotation 检查值是否需要引号
func NeedsQuotation(dt DataType) bool {
	if dt == "" {
		return false // NULL 不需要引号
	}

	switch {
	case IsStringType(dt), IsDateTimeType(dt), dt == TypeJSON:
		return true
	}
	return false
}

// GetDefaultValue 获取类型的默认值
func GetDefaultValue(dt DataType, nullable bool) string {
	if nullable {
		return "NULL"
	}

	switch dt {
	case TypeTinyInt, TypeSmallInt, TypeMediumInt, TypeInt, TypeBigInt:
		return "0"
	case TypeDecimal, TypeNumeric, TypeFloat, TypeDouble:
		return "0.0"
	case TypeBit:
		return "b'0'"
	case TypeChar, TypeVarchar, TypeText, TypeEnum, TypeSet:
		return "''"
	case TypeBlob, TypeBinary, TypeVarbinary:
		return "''"
	case TypeDate:
		return "'0000-00-00'"
	case TypeTime:
		return "'00:00:00'"
	case TypeDatetime, TypeTimestamp:
		return "'0000-00-00 00:00:00'"
	case TypeYear:
		return "0000"
	case TypeJSON:
		return "'{}'"
	case TypeGeometry:
		return "NULL"
	default:
		return "NULL"
	}
}
