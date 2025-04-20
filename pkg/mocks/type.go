package mocks

type MockConfig struct {
	Service      string                 `json:"service"`
	Method       string                 `json:"method"`
	ResponseType string                 `json:"responseType"`
	MockResponse map[string]interface{} `json:"mockResponse"`
	GrpcStatus   int                    `json:"grpcStatus"`
	ErrorString  string                 `json:"errorString"`
	Headers      map[string]string      `json:"headers"`
	DelayMs      int                    `json:"delayMs"`
}
