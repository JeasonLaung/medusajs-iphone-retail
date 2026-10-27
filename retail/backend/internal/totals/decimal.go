package totals

import (
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Decimal arithmetic is immutable. Addition and multiplication are exact;
// division alone rounds half away from zero at 20 decimal places, matching the
// reviewed legacy MathBN / bignumber.js configuration. This is not a provider
// settlement rounding policy or a MySQL storage quantization policy.
type decimal struct{ r *big.Rat }

var ErrInput = errors.New("invalid totals input")
var numeric = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func power(n int) *big.Int    { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }
func integer(n int64) decimal { return decimal{new(big.Rat).SetInt64(n)} }
func parse(s string) (decimal, error) {
	// Bound exponent expansion before allocating big integers. No float parsing,
	// whitespace, fractions, NaN, Infinity, implicit zero or locale punctuation.
	if len(s) == 0 || len(s) > 256 || !numeric.MatchString(s) {
		return decimal{}, ErrInput
	}
	mantissa, exponent := s, 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mantissa = s[:i]
		var err error
		exponent, err = strconv.Atoi(s[i+1:])
		if err != nil || exponent < -64 || exponent > 64 {
			return decimal{}, ErrInput
		}
	}
	unsigned := strings.TrimPrefix(mantissa, "-")
	parts := strings.SplitN(unsigned, ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(parts[0])+len(fraction) > 128 || len(fraction) > 64 {
		return decimal{}, ErrInput
	}
	n, ok := new(big.Int).SetString(parts[0]+fraction, 10)
	if !ok {
		return decimal{}, ErrInput
	}
	if strings.HasPrefix(mantissa, "-") {
		n.Neg(n)
	}
	scale := len(fraction) - exponent
	if scale < 0 {
		n.Mul(n, power(-scale))
		scale = 0
	}
	return decimal{new(big.Rat).SetFrac(n, power(scale))}, nil
}
func (d decimal) add(e decimal) decimal { return decimal{new(big.Rat).Add(d.r, e.r)} }
func (d decimal) sub(e decimal) decimal { return decimal{new(big.Rat).Sub(d.r, e.r)} }
func (d decimal) mul(e decimal) decimal { return decimal{new(big.Rat).Mul(d.r, e.r)} }
func (d decimal) zero() bool            { return d.r.Sign() == 0 }
func (d decimal) div(e decimal) (decimal, error) {
	if e.zero() {
		return decimal{}, ErrInput
	}
	r := new(big.Rat).Quo(d.r, e.r)
	n := roundInteger(new(big.Int).Mul(r.Num(), power(20)), r.Denom())
	return decimal{new(big.Rat).SetFrac(n, power(20))}, nil
}
func roundInteger(n, denominator *big.Int) *big.Int {
	q, remainder := new(big.Int), new(big.Int)
	q.QuoRem(n, denominator, remainder)
	if new(big.Int).Mul(new(big.Int).Abs(remainder), big.NewInt(2)).Cmp(denominator) >= 0 {
		if n.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return q
}
func (d decimal) text() string {
	// Every denominator contains only 2 and 5: inputs are decimal and divisions
	// quantize to 10^20. Find the exact finite representation without a display
	// precision cap that could discard intermediate low digits.
	den := new(big.Int).Set(d.r.Denom())
	two, five := big.NewInt(2), big.NewInt(5)
	a, b := 0, 0
	for new(big.Int).Mod(den, two).Sign() == 0 {
		den.Quo(den, two)
		a++
	}
	for new(big.Int).Mod(den, five).Sign() == 0 {
		den.Quo(den, five)
		b++
	}
	scale := max(a, b)
	s := d.r.FloatString(scale)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		return "0"
	}
	return s
}
func newDecimalEpsilon(scale int) decimal {
	return decimal{new(big.Rat).SetFrac(big.NewInt(1), power(scale))}
}
func newAbs(d decimal) decimal { return decimal{new(big.Rat).Abs(d.r)} }

// RawValue takes precedence over the rounded database/API decimal field.
// Precision is provenance, not permission to round the supplied value again.
type RawValue struct {
	Value     string `json:"value"`
	Precision int    `json:"precision"`
}
type Number struct {
	Decimal string
	Raw     *RawValue
}

func (n Number) value() (decimal, error) {
	if n.Raw != nil {
		if n.Raw.Precision < 1 || n.Raw.Precision > 100 {
			return decimal{}, ErrInput
		}
		return parse(n.Raw.Value)
	}
	return parse(n.Decimal)
}

// Resolve exposes the exact effective scalar for trusted snapshot projections.
// It shares the calculator's raw precedence and validation; no float conversion.
func Resolve(n Number) (string, error) {
	v, err := n.value()
	if err != nil {
		return "", err
	}
	return v.text(), nil
}
