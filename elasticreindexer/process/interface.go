package process

import "bytes"

// ElasticClientHandler defines the behaviour of an elastic search client handler
type ElasticClientHandler interface {
	GetMapping(index string) (*bytes.Buffer, error)
	CreateIndexWithMapping(targetIndex string, body *bytes.Buffer) error
	DoScrollRequestAllDocuments(
		index string,
		body []byte,
		handlerFunc func(responseBytes []byte) error,
	) error
	GetCount(index string) (uint64, error)
	DoesAliasExist(alias string) bool
	DoBulkRequest(buff *bytes.Buffer, index string) error
	DoesIndexExist(index string) bool
	PutAlias(index string, alias string) error
	IsInterfaceNil() bool
}