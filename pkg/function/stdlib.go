package function

import (
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StdlibFunctions returns standard library functions for HCL evaluation.
// These functions are compatible with Terraform/OpenTofu and come from go-cty.
func StdlibFunctions() map[string]function.Function {
	defer perf.Track(nil, "function.StdlibFunctions")()

	funcs := make(map[string]function.Function)

	// Add all function groups.
	addStringFunctions(funcs)
	addCollectionFunctions(funcs)
	addNumericFunctions(funcs)
	addEncodingFunctions(funcs)
	addDateTimeFunctions(funcs)
	addRegexFunctions(funcs)
	addTypeFunctions(funcs)
	addSetFunctions(funcs)
	addLogicFunctions(funcs)
	addComparisonFunctions(funcs)
	addBytesFunctions(funcs)

	return funcs
}

// addStringFunctions adds string manipulation functions.
func addStringFunctions(funcs map[string]function.Function) {
	funcs["chomp"] = stdlib.ChompFunc
	funcs["lower"] = stdlib.LowerFunc
	funcs["upper"] = stdlib.UpperFunc
	funcs["title"] = stdlib.TitleFunc
	funcs["trim"] = stdlib.TrimFunc
	funcs["trimprefix"] = stdlib.TrimPrefixFunc
	funcs["trimsuffix"] = stdlib.TrimSuffixFunc
	funcs["trimspace"] = stdlib.TrimSpaceFunc
	funcs["strlen"] = stdlib.StrlenFunc
	funcs["substr"] = stdlib.SubstrFunc
	funcs["replace"] = stdlib.ReplaceFunc
	funcs["split"] = stdlib.SplitFunc
	funcs["join"] = stdlib.JoinFunc
	funcs["format"] = stdlib.FormatFunc
	funcs["formatlist"] = stdlib.FormatListFunc
	funcs["reverse"] = stdlib.ReverseFunc
	funcs["indent"] = stdlib.IndentFunc
}

// addCollectionFunctions adds collection manipulation functions.
func addCollectionFunctions(funcs map[string]function.Function) {
	funcs["length"] = stdlib.LengthFunc
	funcs["concat"] = stdlib.ConcatFunc
	funcs["contains"] = stdlib.ContainsFunc
	funcs["distinct"] = stdlib.DistinctFunc
	funcs["element"] = stdlib.ElementFunc
	funcs["flatten"] = stdlib.FlattenFunc
	funcs["keys"] = stdlib.KeysFunc
	funcs["values"] = stdlib.ValuesFunc
	funcs["lookup"] = stdlib.LookupFunc
	funcs["merge"] = stdlib.MergeFunc
	funcs["range"] = stdlib.RangeFunc
	funcs["slice"] = stdlib.SliceFunc
	funcs["sort"] = stdlib.SortFunc
	funcs["compact"] = stdlib.CompactFunc
	funcs["chunklist"] = stdlib.ChunklistFunc
	funcs["zipmap"] = stdlib.ZipmapFunc
	funcs["index"] = stdlib.IndexFunc
}

// addNumericFunctions adds numeric functions.
func addNumericFunctions(funcs map[string]function.Function) {
	funcs["abs"] = stdlib.AbsoluteFunc
	funcs["ceil"] = stdlib.CeilFunc
	funcs["floor"] = stdlib.FloorFunc
	funcs["log"] = stdlib.LogFunc
	funcs["max"] = stdlib.MaxFunc
	funcs["min"] = stdlib.MinFunc
	funcs["pow"] = stdlib.PowFunc
	funcs["signum"] = stdlib.SignumFunc
	funcs["parseint"] = stdlib.ParseIntFunc
	funcs["add"] = stdlib.AddFunc
	funcs["subtract"] = stdlib.SubtractFunc
	funcs["multiply"] = stdlib.MultiplyFunc
	funcs["divide"] = stdlib.DivideFunc
	funcs["modulo"] = stdlib.ModuloFunc
	funcs["negate"] = stdlib.NegateFunc
	funcs["int"] = stdlib.IntFunc
}

// addEncodingFunctions adds encoding/decoding functions.
func addEncodingFunctions(funcs map[string]function.Function) {
	funcs["jsonencode"] = stdlib.JSONEncodeFunc
	funcs["jsondecode"] = stdlib.JSONDecodeFunc
	funcs["csvdecode"] = stdlib.CSVDecodeFunc
}

// addDateTimeFunctions adds date/time functions.
func addDateTimeFunctions(funcs map[string]function.Function) {
	funcs["formatdate"] = stdlib.FormatDateFunc
	funcs["timeadd"] = stdlib.TimeAddFunc
}

// addRegexFunctions adds regex functions.
func addRegexFunctions(funcs map[string]function.Function) {
	funcs["regex"] = stdlib.RegexFunc
	funcs["regexall"] = stdlib.RegexAllFunc
}

// addTypeFunctions adds type conversion and coalescing functions.
func addTypeFunctions(funcs map[string]function.Function) {
	funcs["coalesce"] = stdlib.CoalesceFunc
	funcs["coalescelist"] = stdlib.CoalesceListFunc
}

// addSetFunctions adds set manipulation functions.
func addSetFunctions(funcs map[string]function.Function) {
	funcs["setintersection"] = stdlib.SetIntersectionFunc
	funcs["setsubtract"] = stdlib.SetSubtractFunc
	funcs["setunion"] = stdlib.SetUnionFunc
	funcs["setproduct"] = stdlib.SetProductFunc
}

// addLogicFunctions adds boolean logic functions.
func addLogicFunctions(funcs map[string]function.Function) {
	funcs["and"] = stdlib.AndFunc
	funcs["or"] = stdlib.OrFunc
	funcs["not"] = stdlib.NotFunc
}

// addComparisonFunctions adds comparison functions.
func addComparisonFunctions(funcs map[string]function.Function) {
	funcs["equal"] = stdlib.EqualFunc
	funcs["notequal"] = stdlib.NotEqualFunc
	funcs["lessthan"] = stdlib.LessThanFunc
	funcs["lessthanorequalto"] = stdlib.LessThanOrEqualToFunc
	funcs["greaterthan"] = stdlib.GreaterThanFunc
	funcs["greaterthanorequalto"] = stdlib.GreaterThanOrEqualToFunc
}

// addBytesFunctions adds bytes manipulation functions.
func addBytesFunctions(funcs map[string]function.Function) {
	funcs["byteslen"] = stdlib.BytesLenFunc
	funcs["bytesslice"] = stdlib.BytesSliceFunc
}
