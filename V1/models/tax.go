package models

type Tax struct {
	CgstValue int    `json:"cgstValue"`
	SgstValue int    `json:"sgstValue"`
	UgstValue int    `json:"ugstValue"`
	TradeName string `json:"tradeName"`
	Gstin     string `json:"gstin"`
}
