package httpclient

import (
	"testing"
	"time"
	"github.com/stretchr/testify/assert"
	"github.com/aws/amazon-ecs-agent/agent/utils/agent_tls"
	"net/http"
)

func TestNewHttpClient(t *testing.T) {
	expectedResult := New(time.Duration(10),true)
	transport := expectedResult.Transport.(*ecsRoundTripper)
	assert.Equal(t, transport.transport.(*http.Transport).TLSClientConfig.CipherSuites, agent_tls.SupportedCipherSuites)
	assert.Equal(t, transport.transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify, true)
}
