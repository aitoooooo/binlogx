package util

import (
	"fmt"
	"testing"
)

func TestMatch(t *testing.T) {
	// 1. 基础区间
	fmt.Println("==== 1. 基础区间 [0-3] ====")
	pat1 := "srv[0-3]"
	m1, _ := NewRangeMatcher(pat1)
	for _, s := range []string{"srv0", "srv3", "srv4", "srv"} {
		fmt.Printf("pattern=%-10s input=%-6s => %v\n", pat1, s, m1.Match(s))
	}

	// 2. 纯通配符
	fmt.Println("\n==== 2. 纯通配符 * ====")
	pat2 := "*"
	m2, _ := NewRangeMatcher(pat2)
	for _, s := range []string{"a", "A", "1", "_", "abc_123_XYZ", "", "-", "中文"} {
		fmt.Printf("pattern=%-10s input=%-15s => %v\n", pat2, s, m2.Match(s))
	}

	// 3. 混合模式
	fmt.Println("\n==== 3. 混合模式 log_[0-9]_*_v[1-9] ====")
	pat3 := "log_[0-9]_*_v[1-9]"
	m3, _ := NewRangeMatcher(pat3)
	tests3 := []string{
		"log_0_abc_v1",
		"log_9_XYZ_v9",
		"log_10_abc_v1",  // false：第一段 10 不在 0-9
		"log_1_a_b_c_v5", // true
		"log_1__v2",      // false：* 至少一个字符
	}
	for _, s := range tests3 {
		fmt.Printf("pattern=%-25s input=%-20s => %v\n", pat3, s, m3.Match(s))
	}

	// 4. 连续区间
	fmt.Println("\n==== 4. 连续区间 file_[0-2][0-9] ====")
	pat4 := "file_[0-2][0-9]"
	m4, _ := NewRangeMatcher(pat4)
	for _, s := range []string{"file_00", "file_15", "file_29", "file_30", "file_5"} {
		fmt.Printf("pattern=%-15s input=%-10s => %v\n", pat4, s, m4.Match(s))
	}

	// 5. 大区间
	fmt.Println("\n==== 5. 大区间 db_pki_*_file_[0-127] ====")
	pat5 := "db_pki_*_file_[0-127]"
	m5, _ := NewRangeMatcher(pat5)
	tests5 := []string{
		"db_pki_00_file_0",
		"db_pki_abcXYZ123_file_127",
		"db_pki__file_128",   // false：128 超出区间
		"db_pki_文件_file_100", // false：中文不在 \w
	}
	for _, s := range tests5 {
		fmt.Printf("pattern=%-25s input=%-30s => %v\n", pat5, s, m5.Match(s))
	}

	// 6. 边界空串/特殊字符
	fmt.Println("\n==== 6. 边界与特殊字符 ====")
	pat6 := "prefix_*_suffix"
	m6, _ := NewRangeMatcher(pat6)
	for _, s := range []string{
		"prefix_a_suffix",
		"prefix__suffix",
		"prefix_123_suffix",
		"prefix-_suffix", // false：- 不在 \w
		"prefix__suffix", // true
		"prefix_suffix",  // false：* 至少一个
	} {
		fmt.Printf("pattern=%-20s input=%-20s => %v\n", pat6, s, m6.Match(s))
	}

	// 7. 超长组合
	fmt.Println("\n==== 7. 超长组合 ====")
	pat7 := "a*b*c*d*e*"
	m7, _ := NewRangeMatcher(pat7)
	fmt.Printf("pattern=%-15s input=%-40s => %v\n", pat7, "aX_b_Y_c1_d__eZZZ", m7.Match("aX_b_Y_c1_d__eZZZ"))
	fmt.Printf("pattern=%-15s input=%-40s => %v\n", pat7, "aaabbbcccdddeee", m7.Match("aaabbbcccdddeee"))
	fmt.Printf("pattern=%-15s input=%-40s => %v\n", pat7, "abcde", m7.Match("abcde"))

	// 8. 错误模式
	fmt.Println("\n==== 8. 错误模式 ====")
	_, err := NewRangeMatcher("bad[1")
	fmt.Printf("invalid pattern: %v\n", err)
}
