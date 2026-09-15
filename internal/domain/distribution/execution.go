package distribution

// ExecutionProvenance contains only immutable release facts, never credentials.
// Expressions are a closed aggregation enum over an explicitly bound field.
type ExecutionProvenance struct {
	BindingPins     []ExecutionBindingPin `json:"bindingPins"`
	CompilerVersion string                `json:"compilerVersion"`
	Bindings        []ExecutionBinding    `json:"bindings"`
	Relations       []ExecutionRelation   `json:"relations"`
	Joins           []ExecutionJoin       `json:"joins,omitempty"`
	Grains          []ReleasedModelGrain  `json:"grains,omitempty"`
}
type ExecutionBindingPin struct {
	BindingID         string `json:"bindingId"`
	Version           int    `json:"version"`
	DatasetID         string `json:"datasetId"`
	DatasetRevisionID string `json:"datasetRevisionId"`
}
type ExecutionBinding struct {
	AssetID     string `json:"assetId"`
	DatasetID   string `json:"datasetId"`
	FieldID     string `json:"fieldId"`
	Aggregation string `json:"aggregation,omitempty"`
	Expression  string `json:"expression,omitempty"`
}
type ExecutionRelation struct {
	DatasetID         string           `json:"datasetId"`
	DatasetRevisionID string           `json:"datasetRevisionId"`
	SourceID          string           `json:"sourceId"`
	SourceRevisionID  string           `json:"sourceRevisionId"`
	SourceLocator     string           `json:"sourceLocator"`
	AdapterKind       string           `json:"adapterKind"`
	Schema            string           `json:"schema"`
	Relation          string           `json:"relation"`
	Fields            []ExecutionField `json:"fields"`
}
type ExecutionField struct {
	FieldID    string `json:"fieldId"`
	RevisionID string `json:"revisionId"`
	Name       string `json:"name"`
	DataType   string `json:"dataType"`
}
type ExecutionJoin struct {
	Expression     string               `json:"expression"`
	ID             string               `json:"id"`
	Version        int                  `json:"version"`
	LeftDatasetID  string               `json:"leftDatasetId"`
	RightDatasetID string               `json:"rightDatasetId"`
	JoinType       string               `json:"joinType"`
	Cardinality    string               `json:"cardinality"`
	FieldPairs     []ExecutionFieldPair `json:"fieldPairs"`
}
type ExecutionFieldPair struct {
	LeftFieldID  string `json:"leftFieldId"`
	RightFieldID string `json:"rightFieldId"`
}
