package util

import (
	"testing"
)

func TestDataTypeClassification(t *testing.T) {
	tests := []struct {
		dataType DataType
		isNum    bool
		isStr    bool
		isBin    bool
		isDate   bool
	}{
		{TypeInt, true, false, false, false},
		{TypeBigInt, true, false, false, false},
		{TypeDecimal, true, false, false, false},
		{TypeFloat, true, false, false, false},
		{TypeVarchar, false, true, false, false},
		{TypeText, false, true, false, false},
		{TypeEnum, false, true, false, false},
		{TypeBlob, false, false, true, false},
		{TypeBinary, false, false, true, false},
		{TypeDate, false, false, false, true},
		{TypeDatetime, false, false, false, true},
		{TypeTimestamp, false, false, false, true},
		{TypeJSON, false, false, false, false},
		{TypeGeometry, false, false, false, false},
	}

	for _, test := range tests {
		if IsNumericType(test.dataType) != test.isNum {
			t.Errorf("%s: IsNumericType expected %v", test.dataType, test.isNum)
		}
		if IsStringType(test.dataType) != test.isStr {
			t.Errorf("%s: IsStringType expected %v", test.dataType, test.isStr)
		}
		if IsBinaryType(test.dataType) != test.isBin {
			t.Errorf("%s: IsBinaryType expected %v", test.dataType, test.isBin)
		}
		if IsDateTimeType(test.dataType) != test.isDate {
			t.Errorf("%s: IsDateTimeType expected %v", test.dataType, test.isDate)
		}
	}
}

func TestIsUnsignedType(t *testing.T) {
	tests := []struct {
		dataType DataType
		expected bool
	}{
		{TypeTinyInt, true},
		{TypeSmallInt, true},
		{TypeInt, true},
		{TypeBigInt, true},
		{TypeDecimal, true},
		{TypeFloat, true},
		{TypeDouble, true},
		{TypeVarchar, false},
		{TypeText, false},
		{TypeBlob, false},
		{TypeDate, false},
	}

	for _, test := range tests {
		result := IsUnsignedType(test.dataType)
		if result != test.expected {
			t.Errorf("IsUnsignedType(%s): expected %v, got %v", test.dataType, test.expected, result)
		}
	}
}

func TestNeedsQuotation(t *testing.T) {
	tests := []struct {
		dataType DataType
		expected bool
	}{
		{TypeVarchar, true},
		{TypeText, true},
		{TypeDate, true},
		{TypeDatetime, true},
		{TypeJSON, true},
		{TypeInt, false},
		{TypeFloat, false},
		{TypeBlob, false},
		{"", false},
	}

	for _, test := range tests {
		result := NeedsQuotation(test.dataType)
		if result != test.expected {
			t.Errorf("NeedsQuotation(%s): expected %v, got %v", test.dataType, test.expected, result)
		}
	}
}

func TestGetDefaultValue(t *testing.T) {
	tests := []struct {
		dataType DataType
		nullable bool
		expected string
	}{
		{TypeInt, false, "0"},
		{TypeInt, true, "NULL"},
		{TypeDecimal, false, "0.0"},
		{TypeVarchar, false, "''"},
		{TypeDate, false, "'0000-00-00'"},
		{TypeDatetime, false, "'0000-00-00 00:00:00'"},
		{TypeJSON, false, "'{}'"},
		{TypeGeometry, false, "NULL"},
	}

	for _, test := range tests {
		result := GetDefaultValue(test.dataType, test.nullable)
		if result != test.expected {
			t.Errorf("GetDefaultValue(%s, %v): expected %s, got %s", test.dataType, test.nullable, test.expected, result)
		}
	}
}
