package io

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	charm "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMasker_RejectUnboundedCrossLinePatterns(t *testing.T) {
	for _, pattern := range []string{`secret\s+value`, `(?s)secret.*value`, `secret[^x]*value`, `secret\nvalue+`, `secret(?:a|\r)*value`} {
		t.Run(pattern, func(t *testing.T) {
			m := newMasker(&Config{})
			require.ErrorIs(t, m.RegisterPattern(pattern), errUtils.ErrUnboundedMaskPattern)
			require.ErrorIs(t, m.RegisterRegex(regexp.MustCompile(pattern)), errUtils.ErrUnboundedMaskPattern)
			assert.Zero(t, m.Count(), "unsupported patterns must not be accepted as protective masks")
		})
	}
}

func TestMasker_LineLocalPatternsRemainSupported(t *testing.T) {
	for _, pattern := range []string{`(?m)^secret[^\r\n]+$`, `secret[ \t]*value`, `secret\S+value`, `secret\w+`, `secret(?:[a-z]*|[0-9]+)`, `secret(?:\n){0}value`, `(?:^)*secret`, `secret(?:[x]{0})+value`} {
		t.Run(pattern, func(t *testing.T) {
			m := newMasker(&Config{})
			require.NoError(t, m.RegisterPattern(pattern))
			assert.Equal(t, 1, m.Count())
		})
	}
}

func TestMasker_CustomCrossLinePatternWarning(t *testing.T) {
	var output bytes.Buffer
	original := log.Default()
	log.SetDefault(log.NewAtmosLogger(charm.New(&output)))
	t.Cleanup(func() { log.SetDefault(original) })
	cfg := &Config{AtmosConfig: schema.AtmosConfiguration{Settings: schema.AtmosSettings{Terminal: schema.Terminal{Mask: schema.MaskSettings{
		Patterns: []string{`secret\s+value`, `token-[a-z]+`}, Literals: []string{"known-secret"},
	}}}}}
	m := newMasker(&Config{})
	registerCustomMaskPatterns(m, cfg)
	assert.Equal(t, "<MASKED> <MASKED>", m.Mask("known-secret token-abc"))
	assert.Equal(t, 2, m.Count())
	assert.Contains(t, output.String(), "Skipping invalid mask pattern")
	assert.Contains(t, output.String(), "finite maximum match length")
}

func TestMasker_AWSContextualPatternRemainsSupported(t *testing.T) {
	m := newMasker(&Config{})
	m.RegisterAWSAccessKey("AKIA" + strings.Repeat("X", 16))
	assert.Equal(t, 2, m.Count(), "access key literal and contextual secret regex both register")
	for _, spacing := range []string{"", " ", "\t", strings.Repeat(" ", 100)} {
		input := "aws_secret_access_key=" + spacing + strings.Repeat("a", 40)
		assert.Equal(t, MaskReplacement, m.Mask(input))
		assert.Equal(t, MaskReplacement+"\n", streamInChunks(t, m, input+"\n", 20))
	}
}
