package starlark

import (
	"math"
	"math/big"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

const (
	decimalRadix = 10
	// Outside these decimal places, binary64 rounds to itself or signed zero.
	floatUnchangedPlaces = 324
	floatZeroPlaces      = -309
)

// numericSum adds numbers in iteration order, retaining arbitrary-precision integers.
func numericSum(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var iterable starlark.Iterable
	var total starlark.Value = starlark.MakeInt(0)
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "iterable", &iterable, "start?", &total); err != nil {
		return nil, err
	}
	if !isNumber(total) {
		return nil, convert.InvalidArgument("sum: start must be an int or float, got %s", total.Type())
	}
	iter := iterable.Iterate()
	defer iter.Done()
	ctx := threadContext(thread)
	var item starlark.Value
	for iter.Next(&item) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !isNumber(item) {
			return nil, convert.InvalidArgument("sum: items must be int or float, got %s", item.Type())
		}
		var err error
		total, err = starlark.Binary(syntax.PLUS, total, item)
		if err != nil {
			return nil, err
		}
	}
	return total, nil
}

func isNumber(value starlark.Value) bool {
	switch value.(type) {
	case starlark.Int, starlark.Float:
		return true
	default:
		return false
	}
}

// numericRound uses ties-to-even, with an integer result when ndigits is omitted.
func numericRound(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var number starlark.Value
	var digits starlark.Value = starlark.None
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "number", &number, "ndigits?", &digits); err != nil {
		return nil, err
	}
	ndigits := starlark.MakeInt(0)
	if digits != starlark.None {
		var ok bool
		ndigits, ok = digits.(starlark.Int)
		if !ok {
			return nil, convert.InvalidArgument("round: ndigits must be an int or None, got %s", digits.Type())
		}
	}
	switch value := number.(type) {
	case starlark.Int:
		return roundInteger(value, ndigits), nil
	case starlark.Float:
		return roundFloat(float64(value), ndigits, digits == starlark.None)
	default:
		return nil, convert.InvalidArgument("round: number must be an int or float, got %s", number.Type())
	}
}

func roundInteger(value, digits starlark.Int) starlark.Int {
	if digits.Sign() >= 0 {
		return value
	}
	places := new(big.Int).Neg(digits.BigInt())
	// More places than decimal digits always rounds to zero. Bound the exponent
	// before allocating a power of ten, including for arbitrarily large ndigits.
	if !places.IsInt64() || places.Int64() > int64(len(value.String())) {
		return starlark.MakeInt(0)
	}
	scale := new(big.Int).Exp(big.NewInt(decimalRadix), places, nil)
	quotient := roundRatio(new(big.Rat).SetFrac(value.BigInt(), scale))
	return starlark.MakeBigInt(quotient.Mul(quotient, scale))
}

func roundFloat(value float64, digits starlark.Int, integerResult bool) (starlark.Value, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		if integerResult {
			return nil, convert.InvalidArgument("round: cannot round a non-finite float to an int")
		}
		return starlark.Float(value), nil
	}
	if integerResult {
		return starlark.MakeBigInt(roundRatio(new(big.Rat).SetFloat64(value))), nil
	}
	places, fits := digits.Int64()
	if !fits {
		if digits.Sign() > 0 {
			return starlark.Float(value), nil
		}
		return starlark.Float(math.Copysign(0, value)), nil
	}
	// Binary64 cannot be affected by decimal places outside these limits.
	if places >= floatUnchangedPlaces {
		return starlark.Float(value), nil
	}
	if places <= floatZeroPlaces {
		return starlark.Float(math.Copysign(0, value)), nil
	}
	return roundFloatPlaces(value, places)
}

func roundFloatPlaces(value float64, places int64) (starlark.Value, error) {
	exponent := places
	if exponent < 0 {
		exponent = -exponent
	}
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(decimalRadix), big.NewInt(exponent), nil))
	// Scale the exact binary value as a rational. Multiplying floats before
	// rounding can incorrectly turn near-ties such as 2.675 into exact ties.
	scaled := new(big.Rat).SetFloat64(value)
	if places >= 0 {
		scaled.Mul(scaled, scale)
	} else {
		scaled.Quo(scaled, scale)
	}
	result := new(big.Rat).SetInt(roundRatio(scaled))
	if places >= 0 {
		result.Quo(result, scale)
	} else {
		result.Mul(result, scale)
	}
	rounded, _ := result.Float64()
	if math.IsInf(rounded, 0) {
		return nil, convert.InvalidArgument("round: rounded value exceeds the float range")
	}
	if rounded == 0 {
		rounded = math.Copysign(0, value)
	}
	return starlark.Float(rounded), nil
}

// roundRatio rounds an exact rational to the nearest integer, choosing even on ties.
func roundRatio(value *big.Rat) *big.Int {
	numerator := new(big.Int).Abs(value.Num())
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, value.Denom(), remainder)
	comparison := remainder.Lsh(remainder, 1).Cmp(value.Denom())
	if comparison > 0 || (comparison == 0 && quotient.Bit(0) == 1) {
		quotient.Add(quotient, big.NewInt(1))
	}
	if value.Sign() < 0 {
		quotient.Neg(quotient)
	}
	return quotient
}
