package execution_test

import (
	"encoding/json"
	d "github.com/iiwish/semlia/internal/domain/distribution"
	e "github.com/iiwish/semlia/internal/domain/execution"
	s "github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/internal/testsupport/knowledgecase"
	"github.com/iiwish/semlia/pkg/identity"
	"strings"
	"testing"
	"time"
)

func TestWholeKnowledgeCasePlan(t *testing.T) {
	c := knowledgecase.New()
	plan, refusal, err := d.BuildPlan(c.Query, c.Snapshot, identity.SemanticQueryID{}, identity.ResolvedSemanticPlanID{}, nil, time.Now())
	if err != nil || refusal != nil {
		t.Fatalf("positive model plan: %+v %v", refusal, err)
	}
	compiled, err := e.Compile(plan)
	if err != nil || !strings.Contains(compiled.SQL, "NULLIF(COUNT(DISTINCT") || len(compiled.Guards) < 1 || len(plan.Assets) != 10 || plan.Model == nil {
		t.Fatalf("incomplete plan: %+v %v assets=%d", compiled, err, len(plan.Assets))
	}
	for _, test := range []struct {
		name   string
		mutate func(*knowledgecase.Case)
	}{
		{"unknown member", func(c *knowledgecase.Case) { c.Query.Filters[1].Selector.MemberID = "missing" }},
		{"ambiguous model", func(c *knowledgecase.Case) {
			a := c.Snapshot.Assets[len(c.Snapshot.Assets)-1]
			a.AssetID, _ = identity.NewAssetID()
			c.Snapshot.Assets = append(c.Snapshot.Assets, a)
		}},
		{"missing physical pin", func(c *knowledgecase.Case) { c.Snapshot.Execution.BindingPins = nil }},
		{"missing execution snapshot", func(c *knowledgecase.Case) { c.Snapshot.Execution = nil }},
		{"wrong predicate subject", func(c *knowledgecase.Case) {
			a := &c.Snapshot.Assets[5]
			var content map[string]json.RawMessage
			_ = json.Unmarshal(a.Content, &content)
			var spec s.KnowledgeSpec
			_ = json.Unmarshal(content["spec"], &spec)
			r := c.Refs["orders"]
			r.MemberID = "paid_at"
			spec.Predicate.Left.Ref = &r
			content["spec"], _ = json.Marshal(spec)
			a.Content, _ = json.Marshal(content)
		}},
		{"wrong predicate parameter type", func(c *knowledgecase.Case) {
			a := &c.Snapshot.Assets[5]
			var content map[string]json.RawMessage
			_ = json.Unmarshal(a.Content, &content)
			var spec s.KnowledgeSpec
			_ = json.Unmarshal(content["spec"], &spec)
			spec.Parameters[0].Type = "number"
			content["spec"], _ = json.Marshal(spec)
			a.Content, _ = json.Marshal(content)
		}},
		{"wrong dependency revision", func(c *knowledgecase.Case) { c.Snapshot.Assets[0].RevisionID, _ = identity.NewRevisionID() }},
		{"cyclic metric", func(c *knowledgecase.Case) {
			a := &c.Snapshot.Assets[8]
			var content map[string]json.RawMessage
			_ = json.Unmarshal(a.Content, &content)
			var spec s.KnowledgeSpec
			_ = json.Unmarshal(content["spec"], &spec)
			r := c.Refs["average"]
			spec.Expression.Left.Ref = &r
			content["spec"], _ = json.Marshal(spec)
			a.Content, _ = json.Marshal(content)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := knowledgecase.New()
			test.mutate(&c)
			_, refusal, err := d.BuildPlan(c.Query, c.Snapshot, identity.SemanticQueryID{}, identity.ResolvedSemanticPlanID{}, nil, time.Now())
			if err == nil && refusal == nil {
				t.Fatal("unsafe model accepted")
			}
		})
	}
}

func TestWholeKnowledgeCaseRejectsUnsafeJoinsBeforeExecution(t *testing.T) {
	for _, test := range []string{"missing path", "many to many", "ambiguous approved contracts"} {
		t.Run(test, func(t *testing.T) {
			c := knowledgecase.New()
			if test == "missing path" {
				c.Snapshot.Execution.Joins = nil
			} else if test == "many to many" {
				c.Snapshot.Execution.Joins[0].Cardinality = "many_to_many"
			} else {
				join := c.Snapshot.Execution.Joins[0]
				id, _ := identity.NewJoinContractID()
				join.ID = id.String()
				for _, relation := range c.Snapshot.Execution.Relations {
					if relation.DatasetID == join.LeftDatasetID {
						for _, field := range relation.Fields {
							if field.Name == "id" {
								join.FieldPairs = []d.ExecutionFieldPair{{LeftFieldID: field.FieldID, RightFieldID: join.FieldPairs[0].RightFieldID}}
							}
						}
					}
				}
				c.Snapshot.Execution.Joins = append(c.Snapshot.Execution.Joins, join)
				asset := &c.Snapshot.Assets[len(c.Snapshot.Assets)-1]
				var content struct {
					Spec s.KnowledgeSpec `json:"spec"`
				}
				_ = json.Unmarshal(asset.Content, &content)
				content.Spec.JoinContractIDs = append(content.Spec.JoinContractIDs, join.ID)
				asset.Content, _ = json.Marshal(content)
			}
			plan, refusal, err := d.BuildPlan(c.Query, c.Snapshot, identity.SemanticQueryID{}, identity.ResolvedSemanticPlanID{}, nil, time.Now())
			if err != nil || refusal != nil {
				return
			}
			if _, err := e.Compile(plan); err == nil {
				t.Fatal("unsafe join became executable")
			}
			if test == "ambiguous approved contracts" {
				plan.Execution.Joins[0], plan.Execution.Joins[1] = plan.Execution.Joins[1], plan.Execution.Joins[0]
				if _, err := e.Compile(plan); err == nil {
					t.Fatal("reversing ambiguous contracts selected a different role")
				}
			}
		})
	}
}
