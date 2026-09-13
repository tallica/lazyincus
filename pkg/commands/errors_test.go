package commands

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
)

func TestIsConnectionError(t *testing.T) {
	assert.False(t, IsConnectionError(nil))
	assert.False(t, IsConnectionError(api.StatusErrorf(http.StatusNotFound, "Instance not found")))
	assert.True(t, IsConnectionError(&url.Error{
		Op:  "Get",
		URL: "https://192.0.2.1:8443/1.0",
		Err: errors.New("connect: operation timed out"),
	}))
}

func TestIsConnectError(t *testing.T) {
	assert.False(t, IsConnectError(errors.New("something else entirely")))
	assert.True(t, IsConnectError(&ConnectError{
		Remote: "local",
		Err:    errors.New("Can't connect to a local server on a non-Linux system"),
	}))
	assert.True(t, IsConnectError(&url.Error{Op: "Get", URL: "https://192.0.2.1:8443/1.0", Err: errors.New("refused")}))
}

func TestConnectErrorNamesTheRemote(t *testing.T) {
	inner := errors.New("The remote \"nope\" doesn't exist")

	assert.Equal(t, `remote "nope": The remote "nope" doesn't exist`, (&ConnectError{Remote: "nope", Err: inner}).Error())
	assert.Equal(t, inner.Error(), (&ConnectError{Err: inner}).Error())
}
