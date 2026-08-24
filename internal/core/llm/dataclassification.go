package llm

type DataClassification string

const (
	DataClassificationPublic      DataClassification = "public"
	DataClassificationPrivateCode DataClassification = "private_code"
)

func (c DataClassification) Valid() bool {
	return c == DataClassificationPublic || c == DataClassificationPrivateCode
}
