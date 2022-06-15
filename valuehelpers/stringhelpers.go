package valuehelpers

import (
	"regexp"
	"strconv"
	"strings"
)

func InsertEveryN(str string, insert string, n int) string {
	re := regexp.MustCompile(".{" + strconv.Itoa(n) + "}") // Every n chars
	parts := re.FindAllString(str, -1)                     // Split the string into n chars blocks.
	return strings.Join(parts, insert)                     // Put the string back together
}
