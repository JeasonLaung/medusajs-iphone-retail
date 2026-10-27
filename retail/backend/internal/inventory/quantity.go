package inventory

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
)

var decimal = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,46})(\.[0-9]{1,18})?$`)
var ErrQuantity = errors.New("invalid inventory quantity")

// scaled uses integers at scale 18, including MySQL SUM's expanded precision.
func scaled(value string) (*big.Int, error) {
	if !decimal.MatchString(value) {
		return nil, ErrQuantity
	}
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	parts := strings.SplitN(value, ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	n, ok := new(big.Int).SetString(parts[0]+fraction+strings.Repeat("0", 18-len(fraction)), 10)
	if !ok {
		return nil, ErrQuantity
	}
	if negative {
		n.Neg(n)
	}
	return n, nil
}
func fixed(n *big.Int) string {
	sign := ""
	if n.Sign() < 0 {
		sign = "-"
	}
	s := new(big.Int).Abs(n).String()
	if len(s) < 19 {
		s = strings.Repeat("0", 19-len(s)) + s
	}
	return sign + s[:len(s)-18] + "." + s[len(s)-18:]
}
func Difference(a, b string) (string, error) {
	x, err := scaled(a)
	if err != nil {
		return "", err
	}
	y, err := scaled(b)
	if err != nil {
		return "", err
	}
	return fixed(new(big.Int).Sub(x, y)), nil
}
func Equal(a, b string) (bool, error) {
	x, err := scaled(a)
	if err != nil {
		return false, err
	}
	y, err := scaled(b)
	if err != nil {
		return false, err
	}
	return x.Cmp(y) == 0, nil
}
