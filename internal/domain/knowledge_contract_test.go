package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceFragmentContract_ConnectorFieldPreserved(t *testing.T) {
	c := SourceFragmentContract{Connector: "github"}
	assert.Equal(t, "github", c.Connector)
}
