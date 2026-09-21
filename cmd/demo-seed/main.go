// demo-seed installs an isolated, explicitly synthetic local demonstration.
// It is not linked into the server and cannot import into an existing workspace.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	execadapter "github.com/iiwish/semlia/internal/adapters/execution"
	pg "github.com/iiwish/semlia/internal/adapters/postgres"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	cat "github.com/iiwish/semlia/internal/application/catalog"
	distapp "github.com/iiwish/semlia/internal/application/distribution"
	gov "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/demodata"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	dist "github.com/iiwish/semlia/internal/domain/distribution"
	execdomain "github.com/iiwish/semlia/internal/domain/execution"
	g "github.com/iiwish/semlia/internal/domain/governance"
	sem "github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
)

const slug = "demo-202609-commerce"
const demoDatabase = "semlia_demo_202609"
const trace = "20260921000000000000000000000001"
const disclosure = "合成演示流程：由本地种子程序模拟，不代表真人审核或企业生产审批。"

type progress struct {
	Scenario     demodata.Scenario
	Workspace    identity.WorkspaceID `json:",omitzero"`
	Viewer       identity.PrincipalID `json:",omitzero"`
	Author       identity.PrincipalID `json:",omitzero"`
	Reviewer     identity.PrincipalID `json:",omitzero"`
	Account      string
	Password     string
	SourceReady  bool
	Replacements map[string]string
	Targets      map[string]string
	Proposals    map[string]identity.ProposalID
	Done         map[string]bool
	Complete     bool
}

type seed struct {
	ctx       context.Context
	pool      *pg.Pool
	store     *pg.Store
	state     progress
	path      string
	auth      *authapp.Service
	catalog   *cat.Service
	authoring *gov.AuthoringService
	dsn       *url.URL
	maco      bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "demo seed:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "--apply" && os.Args[1] != "--maco-apply") || os.Getenv("SEMLIA_ENV") != "development" {
		return errors.New("use node scripts/dev/seed-knowledge-demo.mjs --apply on the local development environment")
	}
	dsn, err := url.Parse(os.Getenv("SEMLIA_DATABASE_URL"))
	if err != nil {
		return err
	}
	maco := os.Args[1] == "--maco-apply"
	if err = validateTarget(dsn, maco); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	pool, err := pg.Open(ctx, dsn.String())
	if err != nil {
		return err
	}
	defer pool.Close()
	lock, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer lock.Release()
	var locked bool
	if err = lock.QueryRow(ctx, "SELECT pg_try_advisory_lock(20260921, 200)").Scan(&locked); err != nil || !locked {
		return errors.New("another demo import is running")
	}
	defer lock.Exec(context.Background(), "SELECT pg_advisory_unlock(20260921, 200)")
	s := &seed{ctx: ctx, pool: pool, store: pg.NewStore(pool), path: filepath.Join(".semlia", "demo-202609", "state.json"), dsn: dsn, maco: maco}
	if maco {
		s.path = "/var/lib/semlia/demo/state.json"
	}
	if b, e := os.ReadFile(s.path); e == nil {
		if e = json.Unmarshal(b, &s.state); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	} else {
		var count int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM workspaces WHERE slug=$1", slug).Scan(&count); err != nil {
			return err
		}
		if count != 0 && !maco {
			return errors.New("demo workspace exists without ownership receipt; refusing to adopt it")
		}
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM user_accounts WHERE status='active'").Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("exactly one active local account is required; refusing to choose an account")
		}
		if err = pool.QueryRow(ctx, "SELECT id::text FROM user_accounts WHERE status='active'").Scan(&s.state.Account); err != nil {
			return err
		}
		secret := make([]byte, 24)
		if _, err = rand.Read(secret); err != nil {
			return err
		}
		s.state.Password = hex.EncodeToString(secret)
		s.state.Scenario = demodata.New()
		if maco {
			s.state.Password = os.Getenv("SEMLIA_DEMO_PASSWORD")
			if len(s.state.Password) < 24 {
				return errors.New("missing dedicated demo reader credential")
			}
			s.state.Scenario = demodata.NewMaco()
			var workspace, principal string
			if err = pool.QueryRow(ctx, `SELECT w.id::text,m.principal_id::text FROM workspaces w JOIN workspace_memberships m ON m.workspace_id=w.id WHERE w.slug=$1 AND m.user_account_id=$2 AND m.status='active' AND (SELECT count(*) FROM workspaces)=1 AND NOT EXISTS(SELECT 1 FROM semantic_assets) AND NOT EXISTS(SELECT 1 FROM source_connections)`, slug, s.state.Account).Scan(&workspace, &principal); err != nil {
				return errors.New("maco seed requires exactly one freshly bootstrapped, empty demo workspace")
			}
			wid, _ := identity.FromUUID(identity.Workspace, workspace)
			pid, _ := identity.FromUUID(identity.Principal, principal)
			s.state.Workspace, _ = identity.ParseWorkspaceID(wid.String())
			s.state.Viewer, _ = identity.ParsePrincipalID(pid.String())
		}
		s.state.Scenario.Snapshot.WorkspaceID, _ = identity.NewWorkspaceID()
		s.state.Replacements = map[string]string{}
		s.state.Targets = map[string]string{}
		s.state.Proposals = map[string]identity.ProposalID{}
		s.state.Done = map[string]bool{}
		if err = s.save(); err != nil {
			return err
		}
	}
	s.auth = authapp.NewService(s.store, authapp.ClockFunc(time.Now))
	s.catalog = cat.NewService(s.store, cat.ClockFunc(time.Now), cat.WithAuthorizer(s.auth))
	clock := gov.ClockFunc(time.Now)
	policy := gov.NewPolicyService(s.store, clock, gov.WithRuleSource(s.store))
	proposals := gov.NewProposalService(s.store, clock)
	s.authoring = gov.NewAuthoringService(s.store, proposals, gov.NewAgentRunService(s.store, clock), s.auth, clock, gov.WithValidationOrchestrator(gov.NewValidationOrchestrator(proposals, s.store, clock)), gov.WithDecisionRefresher(gov.NewPolicyDecisionTrigger(s.store, policy)))
	if err = s.bootstrap(); err != nil {
		return err
	}
	if err = s.source(); err != nil {
		return err
	}
	if err = demodata.ConfigureSource(s.ctx, s.pool, s.state.Workspace, s.state.Viewer, &s.state.Scenario.Snapshot, s.state.Password, []byte(os.Getenv("SEMLIA_SECRET_KEY"))); err != nil {
		return err
	}
	for _, asset := range s.state.Scenario.Snapshot.Assets {
		if asset.AssetType == sem.AnalysisModel {
			if err = s.physical(); err != nil {
				return err
			}
		}
		if err = s.asset(asset); err != nil {
			return fmt.Errorf("%s: %w", asset.Address, err)
		}
	}
	if err = s.relations(); err != nil {
		return err
	}
	if err = s.verify(); err != nil {
		return err
	}
	s.state.Complete = true
	if err = s.save(); err != nil {
		return err
	}
	source, _ := identity.FromUUID(identity.SourceConnection, s.state.Scenario.Snapshot.Execution.Relations[0].SourceID)
	public := map[string]any{"workspaceId": s.state.Workspace.String(), "sourceId": source.String(), "dsnEnv": "SEMLIA_EXECUTION_DSN_DEMO_202609", "query": remap(s.state.Scenario.Query, s.state.Replacements), "expected": 200, "disclosure": disclosure}
	bytes, _ := json.MarshalIndent(public, "", "  ")
	if err = os.WriteFile(filepath.Join(filepath.Dir(s.path), "demo.json"), bytes, 0600); err != nil {
		return err
	}
	fmt.Println("Synthetic demo ready: 10 knowledge assets, 7 orders, 4 customers; verified average = 200 CNY/order.")
	return nil
}

func (s *seed) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(s.path+".tmp", b, 0600); err != nil {
		return err
	}
	return os.Rename(s.path+".tmp", s.path)
}

func (s *seed) bootstrap() error {
	if s.state.Workspace.IsZero() {
		w, err := s.catalog.CreateWorkspace(s.ctx, slug, "合成演示 · 订单语义平台", trace)
		if err != nil {
			return err
		}
		s.state.Workspace = w.ID
		if err = s.save(); err != nil {
			return err
		}
	}
	var got string
	if err := s.pool.QueryRow(s.ctx, "SELECT slug FROM workspaces WHERE id=$1", s.state.Workspace.UUID()).Scan(&got); err != nil || got != slug {
		return errors.New("workspace ownership receipt mismatch")
	}
	for _, actor := range []struct {
		p          *identity.PrincipalID
		name, role string
	}{{&s.state.Viewer, "演示工作区管理员", "workspace_admin"}, {&s.state.Author, "合成演示角色：知识编写者（非真实账号）", "asset_owner"}, {&s.state.Reviewer, "合成演示角色：审核者（非真实账号）", "reviewer"}} {
		if !actor.p.IsZero() {
			continue
		}
		id, _ := identity.NewPrincipalID()
		if _, err := s.store.CreatePrincipal(s.ctx, auth.Principal{ID: id, WorkspaceID: s.state.Workspace, Kind: auth.PrincipalHuman, DisplayName: actor.name, Status: auth.PrincipalActive, CreatedAt: time.Now()}); err != nil {
			return err
		}
		binding, _ := identity.NewBindingID()
		if _, err := s.store.CreateRoleBinding(s.ctx, auth.RoleBinding{ID: binding, PrincipalID: id, RoleID: actor.role, ScopeType: auth.ScopeWorkspace, ScopeID: s.state.Workspace.UUID()}); err != nil {
			return err
		}
		*actor.p = id
		if err := s.save(); err != nil {
			return err
		}
	}
	id, _ := identity.NewMembershipID()
	_, err := s.pool.Exec(s.ctx, `INSERT INTO workspace_memberships(id,workspace_id,user_account_id,principal_id,admitted_by_principal_id) VALUES($1,$2,$3,$4,$4) ON CONFLICT(workspace_id,user_account_id) DO NOTHING`, id.UUID(), s.state.Workspace.UUID(), s.state.Account, s.state.Viewer.UUID())
	return err
}

func (s *seed) source() error {
	if s.maco {
		return s.macoSource()
	}
	admin, err := pgx.Connect(s.ctx, s.dsn.String())
	if err != nil {
		return err
	}
	defer admin.Close(s.ctx)
	var exists bool
	if err = admin.QueryRow(s.ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", demoDatabase).Scan(&exists); err != nil {
		return err
	}
	marker := "Semlia synthetic demo 202609; owner=" + s.state.Workspace.String()
	if !exists {
		if _, err = admin.Exec(s.ctx, "CREATE DATABASE "+demoDatabase); err != nil {
			return err
		}
		if _, err = admin.Exec(s.ctx, "COMMENT ON DATABASE "+demoDatabase+" IS "+literal(marker)); err != nil {
			return err
		}
	} else {
		var comment string
		if err = admin.QueryRow(s.ctx, "SELECT COALESCE(shobj_description(oid,'pg_database'),'') FROM pg_database WHERE datname=$1", demoDatabase).Scan(&comment); err != nil {
			return err
		}
		if comment != marker {
			return errors.New("synthetic database ownership mismatch; refusing reuse")
		}
	}
	if err = admin.QueryRow(s.ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)", demoDatabase).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = admin.Exec(s.ctx, "CREATE ROLE "+demoDatabase+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD "+literal(s.state.Password)); err != nil {
			return err
		}
		if _, err = admin.Exec(s.ctx, "COMMENT ON ROLE "+demoDatabase+" IS "+literal(marker)); err != nil {
			return err
		}
	} else {
		var comment string
		if err = admin.QueryRow(s.ctx, "SELECT COALESCE(shobj_description(oid,'pg_authid'),'') FROM pg_roles WHERE rolname=$1", demoDatabase).Scan(&comment); err != nil {
			return err
		}
		if comment != marker {
			return errors.New("synthetic role ownership mismatch; refusing reuse")
		}
	}
	u := *s.dsn
	u.Path = "/" + demoDatabase
	source, err := pgx.Connect(s.ctx, u.String())
	if err != nil {
		return err
	}
	defer source.Close(s.ctx)
	if err = source.QueryRow(s.ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)", demoDatabase).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		tx, e := source.Begin(s.ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(s.ctx)
		if _, e = tx.Exec(s.ctx, s.state.Scenario.DataSQL); e != nil {
			return e
		}
		if e = tx.Commit(s.ctx); e != nil {
			return e
		}
	}
	// These privileges concern only the owned synthetic database, never the application database.
	if _, err = source.Exec(s.ctx, "REVOKE ALL ON DATABASE "+demoDatabase+" FROM PUBLIC; REVOKE CREATE ON SCHEMA public FROM PUBLIC; GRANT CONNECT ON DATABASE "+demoDatabase+" TO "+demoDatabase+"; GRANT USAGE ON SCHEMA "+demoDatabase+" TO "+demoDatabase+"; GRANT SELECT ON ALL TABLES IN SCHEMA "+demoDatabase+" TO "+demoDatabase); err != nil {
		return err
	}
	if _, err = source.Exec(s.ctx, "ALTER ROLE "+demoDatabase+" IN DATABASE "+demoDatabase+" SET log_min_error_statement = 'panic'"); err != nil {
		return err
	}
	if !s.state.SourceReady {
		s.state.Scenario.Snapshot.WorkspaceID = s.state.Workspace
		if err = demodata.SeedSource(s.ctx, s.pool, s.state.Workspace, s.state.Viewer, &s.state.Scenario.Snapshot); err != nil {
			return err
		}
		s.state.SourceReady = true
		return s.save()
	}
	return nil
}

func (s *seed) asset(asset dist.ReleasedAsset) error {
	key := asset.Address
	if s.state.Done[key] {
		return nil
	}
	var content map[string]any
	if err := json.Unmarshal(asset.Content, &content); err != nil {
		return err
	}
	content = remap(content, s.state.Replacements).(map[string]any)
	definition, _ := content["definition"].(string)
	content["name"] = asset.Name
	content["definition"] = definition + "（演示草稿，待模拟流程确认）"
	body, _ := json.Marshal(content)
	var id identity.AssetID
	if target := s.state.Targets[key]; target != "" {
		id, _ = identity.ParseAssetID(target)
	} else {
		created, err := s.catalog.CreateAsset(s.ctx, cat.CreateAssetRequest{WorkspaceID: s.state.Workspace, Address: key, AssetType: asset.AssetType, Lifecycle: "active", SchemaVersion: "1.0.0", Content: body, CreatedBy: s.state.Author.String(), PrincipalRef: s.state.Author.String(), TraceID: trace})
		if err != nil {
			return err
		}
		id = created.ID
		s.state.Targets[key] = id.String()
		if err = s.save(); err != nil {
			return err
		}
	}
	detail, err := s.store.GetCatalogAsset(s.ctx, s.state.Workspace, id)
	if err != nil {
		return err
	}
	if err = s.publish(key, g.TargetSemanticAsset, id.String(), detail.CurrentRevision.ID, "definition", content["definition"], definition); err != nil {
		return err
	}
	snapshot, err := s.store.CurrentReleaseSnapshot(s.ctx, s.state.Workspace)
	if err != nil {
		return err
	}
	for _, a := range snapshot.Assets {
		if a.AssetID == id {
			s.state.Replacements[asset.AssetID.String()] = id.String()
			s.state.Replacements[asset.RevisionID.String()] = a.RevisionID.String()
			// Each semantic reference must preserve the release that actually published its revision.
			s.state.Replacements["release:"+asset.AssetID.String()] = snapshot.ReleaseID.String()
			s.state.Done[key] = true
			fmt.Println("Published synthetic knowledge:", asset.Name)
			return s.save()
		}
	}
	return errors.New("published asset missing from release")
}

func (s *seed) publish(key string, target g.TargetObjectType, id string, revision identity.RevisionID, field string, before, after any) error {
	p := s.state.Proposals[key]
	if p.IsZero() {
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(after)
		op := g.ChangeUpdate
		if before == nil {
			op = "add"
			b = nil
		}
		created, err := s.authoring.CreateProposal(s.ctx, gov.CreateAuthoringProposalRequest{WorkspaceID: s.state.Workspace, TargetType: target, TargetObjectID: id, BaseRevisionID: revision, Title: "合成演示发布：" + key, Summary: disclosure, Reason: "装载设计稿中的固定样本；仅用于隔离工作区演示。", CreatedBy: s.state.Author.String(), PrincipalRef: s.state.Author.String(), TraceID: trace, ChangeSet: []gov.ChangeSetItemInput{{FieldPath: field, Op: op, BeforeValue: b, AfterValue: a, BeforeDigest: digest(b), AfterDigest: digest(a)}}})
		if err != nil {
			return err
		}
		p = created.Proposal.ID
		s.state.Proposals[key] = p
		if err = s.save(); err != nil {
			return err
		}
	}
	proposals := gov.NewProposalService(s.store, gov.ClockFunc(time.Now))
	for i := 0; i < 180; i++ {
		proposal, err := proposals.GetProposal(s.ctx, s.state.Workspace, p)
		if err != nil {
			return err
		}
		switch proposal.State {
		case g.ProposalReleased:
			return nil
		case g.ProposalDraft, g.ProposalProposed:
			if _, err = s.authoring.SubmitProposal(s.ctx, gov.SubmitAuthoringProposalRequest{WorkspaceID: s.state.Workspace, ProposalID: p, PrincipalRef: s.state.Author.String(), TraceID: trace}); err != nil {
				return err
			}
		case g.ProposalValidating:
			select {
			case <-s.ctx.Done():
				return s.ctx.Err()
			case <-time.After(time.Second):
			}
		case g.ProposalInReview:
			var count int
			if err = s.pool.QueryRow(s.ctx, "SELECT count(*) FROM reviews WHERE workspace_id=$1 AND proposal_id=$2", s.state.Workspace.UUID(), p.UUID()).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				if _, err = s.authoring.ReviewProposal(s.ctx, gov.ReviewProposalRequest{WorkspaceID: s.state.Workspace, ProposalID: p, Decision: gov.ReviewCommandDecision("approve"), Reason: disclosure, PrincipalRef: s.state.Reviewer.String(), TraceID: trace}); err != nil {
					return err
				}
			}
			_, err = s.authoring.Publishing().PublishProposal(s.ctx, gov.PublishProposalRequest{WorkspaceID: s.state.Workspace, ProposalID: p, PrincipalRef: s.state.Viewer.String(), TraceID: trace})
			return err
		default:
			return fmt.Errorf("demo proposal stopped in %s; inspect validation findings", proposal.State)
		}
	}
	return errors.New("validation timed out; keep native worker running and retry")
}

func (s *seed) physical() error {
	service := gov.NewGovernedObjectService(s.store, gov.ClockFunc(time.Now))
	for i, b := range s.state.Scenario.Snapshot.Bindings {
		key := fmt.Sprintf("binding-%d", i)
		if s.state.Done[key] {
			continue
		}
		id := s.state.Targets[key]
		if id == "" {
			asset, err := identity.ParseAssetID(s.state.Replacements[b.AssetID.String()])
			if err != nil {
				return err
			}
			obj, err := service.Create(s.ctx, gov.CreateGovernedObjectRequest{WorkspaceID: s.state.Workspace, CreatedBy: s.state.Author.String(), Object: g.GovernedObject{Type: g.TargetPhysicalBinding, PhysicalBinding: &g.PhysicalBinding{WorkspaceID: s.state.Workspace, AssetID: asset, DatasetID: b.DatasetID, Content: json.RawMessage(`{}`)}}})
			if err != nil {
				return err
			}
			id = obj.PhysicalBinding.ID.String()
			s.state.Targets[key] = id
			if err = s.save(); err != nil {
				return err
			}
		}
		if err := s.publish(key, g.TargetPhysicalBinding, id, identity.RevisionID{}, "notes", nil, disclosure); err != nil {
			return err
		}
		s.state.Done[key] = true
		if err := s.save(); err != nil {
			return err
		}
	}
	for i, j := range s.state.Scenario.Snapshot.Execution.Joins {
		key := fmt.Sprintf("join-%d", i)
		if s.state.Done[key] {
			continue
		}
		id := s.state.Targets[key]
		if id == "" {
			left, _ := identity.FromUUID(identity.PhysicalDataset, j.LeftDatasetID)
			right, _ := identity.FromUUID(identity.PhysicalDataset, j.RightDatasetID)
			ld, _ := identity.ParsePhysicalDatasetID(left.String())
			rd, _ := identity.ParsePhysicalDatasetID(right.String())
			contract := &g.JoinContract{WorkspaceID: s.state.Workspace, LeftDatasetID: ld, RightDatasetID: rd, JoinType: g.JoinLeft, Cardinality: g.CardinalityManyToOne, JoinExpression: "field_pairs_equal/v1", Content: json.RawMessage(`{}`)}
			for _, pair := range j.FieldPairs {
				l, _ := identity.FromUUID(identity.PhysicalField, pair.LeftFieldID)
				r, _ := identity.FromUUID(identity.PhysicalField, pair.RightFieldID)
				lf, _ := identity.ParsePhysicalFieldID(l.String())
				rf, _ := identity.ParsePhysicalFieldID(r.String())
				contract.LeftFieldRefs = append(contract.LeftFieldRefs, lf)
				contract.RightFieldRefs = append(contract.RightFieldRefs, rf)
			}
			obj, err := service.Create(s.ctx, gov.CreateGovernedObjectRequest{WorkspaceID: s.state.Workspace, CreatedBy: s.state.Author.String(), Object: g.GovernedObject{Type: g.TargetJoinContract, JoinContract: contract}})
			if err != nil {
				return err
			}
			id = obj.JoinContract.ID.String()
			s.state.Targets[key] = id
			if err = s.save(); err != nil {
				return err
			}
		}
		if err := s.publish(key, g.TargetJoinContract, id, identity.RevisionID{}, "notes", nil, disclosure); err != nil {
			return err
		}
		s.state.Replacements[j.ID] = id
		s.state.Done[key] = true
		if err := s.save(); err != nil {
			return err
		}
	}
	return nil
}

func (s *seed) verify() error {
	var query dist.SemanticQueryInput
	b, _ := json.Marshal(remap(s.state.Scenario.Query, s.state.Replacements))
	if err := json.Unmarshal(b, &query); err != nil {
		return err
	}
	plans := distapp.NewService(s.store, s.auth, distapp.ClockFunc(time.Now))
	result, err := plans.Resolve(s.ctx, distapp.ResolveRequest{WorkspaceID: s.state.Workspace, Input: query, Channel: "system", IdempotencyKey: "synthetic-demo-202609-proof", PrincipalRef: s.state.Viewer.String(), TraceID: trace})
	if err != nil {
		return err
	}
	if result.Plan == nil {
		return fmt.Errorf("demo did not resolve: %+v", result)
	}
	compiled, err := execdomain.Compile(*result.Plan)
	if err != nil {
		return err
	}
	source, _ := identity.FromUUID(identity.SourceConnection, s.state.Scenario.Snapshot.Execution.Relations[0].SourceID)
	config, _ := json.Marshal([]execadapter.SourceReference{{WorkspaceID: s.state.Workspace.String(), SourceID: source.String(), DSNEnv: "SEMLIA_EXECUTION_DSN_DEMO_202609"}})
	u := s.sourceDSN()
	adapter, err := execadapter.NewPostgresWithLookup(string(config), func(string) (string, bool) { return u.String(), true })
	if err != nil {
		return err
	}
	output := adapter.WithLocalPlaintext().Execute(s.ctx, s.state.Workspace, compiled, execdomain.DefaultLimits())
	proof, _ := json.MarshalIndent(output, "", "  ")
	if err = os.WriteFile(filepath.Join(filepath.Dir(s.path), "execution-proof.json"), proof, 0600); err != nil {
		return err
	}
	if output.ErrorCode != "" {
		return fmt.Errorf("synthetic execution failed: %s", output.ErrorCode)
	}
	if len(output.Rows) != 1 || len(output.Rows[0]) != 1 {
		return errors.New("expected one aggregate cell")
	}
	value, ok := new(big.Rat).SetString(fmt.Sprint(output.Rows[0][0]))
	if !ok || value.Cmp(big.NewRat(200, 1)) != 0 {
		return fmt.Errorf("expected average 200; got %v", output.Rows)
	}
	return nil
}

func validateTarget(dsn *url.URL, maco bool) error {
	if maco {
		if dsn.Hostname() == "postgresql" && dsn.Port() == "5432" && dsn.Path == "/semlia_test" && dsn.User.Username() == "semlia_test_migrator" {
			return nil
		}
	} else if dsn.Hostname() == "127.0.0.1" && dsn.Port() == "5433" && dsn.Path == "/semlia" {
		return nil
	}
	return errors.New("demo target is not the explicitly selected owned deployment")
}

func (s *seed) sourceDSN() url.URL {
	u := *s.dsn
	if s.maco {
		u.User = url.UserPassword("semlia_test_demo", s.state.Password)
	} else {
		u.Path = "/" + demoDatabase
		u.User = url.UserPassword(demoDatabase, s.state.Password)
	}
	u.RawQuery = "sslmode=disable"
	return u
}

func (s *seed) macoSource() error {
	var exists bool
	if err := s.pool.QueryRow(s.ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)", demodata.Schema).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		tx, err := s.pool.Begin(s.ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(s.ctx)
		if _, err = tx.Exec(s.ctx, s.state.Scenario.DataSQL); err != nil {
			return err
		}
		if _, err = tx.Exec(s.ctx, "GRANT USAGE ON SCHEMA semlia_demo_202609 TO semlia_test_demo; GRANT SELECT ON ALL TABLES IN SCHEMA semlia_demo_202609 TO semlia_test_demo"); err != nil {
			return err
		}
		if err = tx.Commit(s.ctx); err != nil {
			return err
		}
	}
	s.state.Scenario.Snapshot.WorkspaceID = s.state.Workspace
	if err := demodata.SeedSource(s.ctx, s.pool, s.state.Workspace, s.state.Viewer, &s.state.Scenario.Snapshot); err != nil {
		return err
	}
	s.state.SourceReady = true
	return s.save()
}

func (s *seed) relations() error {
	// These catalog edges visualize the same dependencies as the typed specs.
	edges := [][3]string{{"paid", "describes", "orders"}, {"old", "describes", "customers"}, {"order_data", "describes", "orders"}, {"customer_data", "describes", "customers"}, {"amount", "measures", "orders"}, {"count", "measures", "orders"}, {"average", "derived_from", "amount"}, {"average", "derived_from", "count"}, {"amount", "filters_by", "paid"}, {"count", "filters_by", "paid"}, {"model", "filters_by", "old"}}
	for _, name := range []string{"orders", "customers", "order_data", "customer_data", "paid", "old", "amount", "count", "average"} {
		edges = append(edges, [3]string{"model", "contains", name})
	}
	for _, edge := range edges {
		left, err := identity.ParseAssetID(s.state.Replacements[s.state.Scenario.Refs[edge[0]].AssetID])
		if err != nil {
			return err
		}
		right, err := identity.ParseAssetID(s.state.Replacements[s.state.Scenario.Refs[edge[2]].AssetID])
		if err != nil {
			return err
		}
		var exists bool
		if err = s.pool.QueryRow(s.ctx, "SELECT EXISTS(SELECT 1 FROM semantic_relations WHERE workspace_id=$1 AND subject_asset_id=$2 AND predicate=$3 AND object_asset_id=$4 AND assertion_state='asserted')", s.state.Workspace.UUID(), left.UUID(), edge[1], right.UUID()).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		id, _ := identity.NewRelationID()
		plane := sem.RelationPlane("semantic")
		if edge[1] == "derived_from" {
			plane = "dependency"
		}
		if _, err = s.store.CreateRelation(s.ctx, sem.RelationRecord{ID: id, WorkspaceID: s.state.Workspace, SubjectAssetID: left, ObjectAssetID: right, Predicate: sem.RelationPredicate(edge[1]), Plane: plane, AssertionState: sem.AssertionState("asserted"), CreatedBy: s.state.Author.String()}); err != nil {
			return err
		}
	}
	return nil
}

func literal(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }
func digest(v []byte) string {
	if len(v) == 0 {
		return ""
	}
	sum := sha256.Sum256(v)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Rebind structured fixture references to actual publication identities.
func remap(value any, replacements map[string]string) any {
	b, _ := json.Marshal(value)
	var tree any
	_ = json.Unmarshal(b, &tree)
	var visit func(any) any
	visit = func(v any) any {
		switch x := v.(type) {
		case string:
			if replacement, ok := replacements[x]; ok {
				return replacement
			}
		case []any:
			for i := range x {
				x[i] = visit(x[i])
			}
		case map[string]any:
			original, _ := x["assetId"].(string)
			for key, item := range x {
				x[key] = visit(item)
			}
			if _, ok := x["releaseId"]; ok && original != "" {
				if release := replacements["release:"+original]; release != "" {
					x["releaseId"] = release
				}
			}
		}
		return v
	}
	return visit(tree)
}
