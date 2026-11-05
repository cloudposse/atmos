package cast

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/cloudposse/atmos/pkg/castplan"
)

type testHost struct {
	plan      *castplan.Plan
	generated bool
	validated bool
}

func (h *testHost) Check(_ *starlark.Thread, p *castplan.Plan) error { h.plan = p; return nil }
func (h *testHost) Generate(_ *starlark.Thread, p *castplan.Plan, _ starlark.Callable) (string, error) {
	h.generated = true
	return "demo.cast", nil
}

func (h *testHost) Validate(_ *starlark.Thread, p *castplan.Plan, _ starlark.Callable) (string, error) {
	h.validated = true
	return "demo.cast", nil
}

func evalCast(source string, h *testHost) error {
	_, err := starlark.ExecFileOptions(&syntax.FileOptions{TopLevelControl: true}, &starlark.Thread{Name: "test"}, "test.star", source, starlark.StringDict{"cast": New(h)})
	return err
}

func TestRecorderDeclaration(t *testing.T) {
	h := &testHost{}
	require.NoError(t, evalCast(`rec=cast.recorder(output="demo.cast", working_directory="fixture")
rec.comment("one\ntwo")
rec.run("actual", display="visible", expect=["literal",cast.regex("x+")], reject="secret",exit_code=3)
rec.terminal.send("command")
rec.terminal.wait(cast.regex("ready.*"))
rec.terminal.close()
rec.cd("nested")
rec.run("next")
rec.validate()`, h))
	require.True(t, h.validated)
	require.False(t, h.generated)
	require.Len(t, h.plan.Actions, 5)
	require.Equal(t, "# one\n# two", h.plan.Actions[0].Fields["text"])
	require.Equal(t, 3, h.plan.Actions[1].ExitCode)
	require.True(t, h.plan.Actions[1].Expect[1].Regex)
	require.Equal(t, "actual", h.plan.Actions[1].Fields["command"])
	require.Len(t, h.plan.Actions[2].Children, 3)
	require.Equal(t, "ready.*", h.plan.Actions[2].Children[2].Fields["regex"])
	require.Contains(t, h.plan.Actions[4].Fields["working_directory"], "nested")
}

func TestRecorderGroupsAreDeclarations(t *testing.T) {
	h := &testHost{}
	require.NoError(t, evalCast(`def setup(c):
    c.run("setup")
def cleanup(c):
    c.run("cleanup")
rec=cast.recorder(output="demo.cast")
rec.setup(setup)
rec.cleanup(cleanup)
rec.hidden(setup)
rec.terminal.send("first")
rec.terminal.close()
rec.terminal.send("second")
rec.generate()`, h))
	require.Len(t, h.plan.Setup, 1)
	require.Len(t, h.plan.Cleanup, 1)
	require.Equal(t, "hidden", h.plan.Actions[0].Kind)
	require.Len(t, h.plan.Actions, 3)
}

func TestRecorderInvalidDeclarations(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{`rec=cast.recorder(output="x"); rec.terminal.send("x"); rec.run("x")`, "close rec.terminal"},
		{`rec=cast.recorder(output="x"); rec.run("x",expect=123)`, "expect/reject"},
		{`rec=cast.recorder(output="x"); rec.run("x",exit_code=-1)`, "exit_code"},
		{`rec=cast.recorder(output="x"); rec.run("x",command="y")`, "duplicate"},
		{`cast.regex("[")`, "invalid cast regex"},
		{`cast.recorder(steps=[])`, "own actions"},
		{`rec=cast.recorder(output="x"); rec.generate(); rec.run("x")`, "finalized"},
		{`rec=cast.recorder(output="x"); rec.terminal.close("x")`, "takes no arguments"},
	} {
		t.Run(tc.want, func(t *testing.T) { require.ErrorContains(t, evalCast(tc.source, &testHost{}), tc.want) })
	}
}

func TestRecorderPublicActionsAndOptions(t *testing.T) {
	h := &testHost{}
	require.NoError(t, evalCast(`def policy(text):
    return None
rec=cast.recorder(title="demo",output={"cast":"demo.cast","png":"final.png"}, policy=policy, replacements={"/checkout":"/repo"})
rec.simulate("display only",rate="0s")
rec.prompt()
rec.pause("1ms")
rec.screenshot("frame.png")
rec.step("container",action="run",with_={"image":"example"})
rec.run("command", expect=("first",cast.regex("second")), reject=["third"])
rec.terminal.write("partial")
rec.terminal.key("enter",repeat=2)
rec.terminal.hide()
rec.terminal.wait(regex="ready",timeout="1s")
rec.terminal.show()
rec.terminal.pause("1ms")
rec.terminal.screenshot("terminal.png")
rec.generate()`, h))
	require.True(t, h.generated)
	require.Equal(t, "/repo", h.plan.Replacements["/checkout"])
	require.Equal(t, "prompt", h.plan.Actions[1].Fields["mode"])
	require.Contains(t, h.plan.Actions[4].Fields, "with")
	require.Len(t, h.plan.Actions[6].Children, 7)
}

func TestRecorderRejectsInvalidArguments(t *testing.T) {
	cases := []struct{ source, want string }{
		{`cast.recorder("x")`, "keyword"},
		{`cast.recorder(output="x",policy="bad")`, "policy must"},
		{`cast.recorder(output="x",replacements=[])`, "replacements"},
		{`cast.recorder(output="x",replacements={"":"x"})`, "replacements"},
		{`cast.recorder(output="x",replacements={"x":3})`, "replacements"},
		{`cast.regex(1)`, "string"},
		{`rec=cast.recorder(output="x");rec.run("x",reject=[[]])`, "lists"},
		{`rec=cast.recorder(output="x");rec.run("x",exit_code="3")`, "int"},
		{`rec=cast.recorder(output="x");rec.run("x",exit_code=256)`, "exit_code"},
		{`rec=cast.recorder(output="x");rec.run("x","y")`, "at most"},
		{`rec=cast.recorder(output="x");rec.prompt("x")`, "keyword"},
		{`rec=cast.recorder(output="x");rec.pause("1s",expect="x")`, "belong on rec.run"},
		{`rec=cast.recorder(output="x");rec.comment("")`, "requires text"},
		{`rec=cast.recorder(output="x");rec.cd("")`, "requires a path"},
		{`rec=cast.recorder(output="x");rec.generate("x")`, "takes no arguments"},
		{`rec=cast.recorder(output="x");rec.setup("bad")`, "callable"},
		{`rec=cast.recorder(output="x");rec.terminal.send("")`, "requires text"},
		{`rec=cast.recorder(output="x");rec.missing()`, "missing"},
		{`rec=cast.recorder(output="x");rec.terminal.missing()`, "missing"},
		{`def group(c):
    c.setup(group)
rec=cast.recorder(output="x")
rec.hidden(group)`, "root recorder"},
		{`def group(c):
    c.generate()
rec=cast.recorder(output="x")
rec.hidden(group)`, "root recorder"},
		{`def group(c):
    fail("group failed")
rec=cast.recorder(output="x")
rec.hidden(group)`, "group failed"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) { require.ErrorContains(t, evalCast(tc.source, &testHost{}), tc.want) })
	}
}

func TestRecorderValuesFreezeAndDisplay(t *testing.T) {
	r := &recorder{plan: &castplan.Plan{}}
	require.Equal(t, "cast.recorder", r.String())
	require.Equal(t, "cast.recorder", r.Type())
	require.True(t, bool(r.Truth()))
	_, err := r.Hash()
	require.Error(t, err)
	value, err := r.Attr("terminal")
	require.NoError(t, err)
	terminal := value.(*terminal)
	require.Equal(t, "cast.terminal", terminal.String())
	require.Equal(t, "cast.terminal", terminal.Type())
	require.True(t, bool(terminal.Truth()))
	_, err = terminal.Hash()
	require.Error(t, err)
	terminal.Freeze()
	require.ErrorContains(t, r.mutable(), "frozen")
	matcher := &regexMatcher{pattern: "x"}
	require.Equal(t, `cast.regex("x")`, matcher.String())
	require.Equal(t, "cast.regex", matcher.Type())
	matcher.Freeze()
	require.True(t, bool(matcher.Truth()))
	_, err = matcher.Hash()
	require.NoError(t, err)
}
