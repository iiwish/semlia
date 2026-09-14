package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	execadapter "github.com/iiwish/semlia/internal/adapters/execution"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	distapp "github.com/iiwish/semlia/internal/application/distribution"
	execapp "github.com/iiwish/semlia/internal/application/execution"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	dist "github.com/iiwish/semlia/internal/domain/distribution"
	execdomain "github.com/iiwish/semlia/internal/domain/execution"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
)

func TestExecutionPublicationCarriesOnlyPublishedPins(t *testing.T) {
	env := newFixture(t)
	w := createWorkspace(t, env.pool, "execution-publication")
	author := createPrincipalWithRoles(t, env, w, "author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, env, w, "publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, env, w, "reviewer")
	first, firstRevision := env.createAsset(t, w)
	second, secondRevision := env.createAssetWithAddress(t, w, "finance.second")
	draft, _ := env.createAssetWithAddress(t, w, "finance.unpublished")
	for _, asset := range []struct {
		id  string
		rev string
	}{{first.String(), firstRevision.String()}, {second.String(), secondRevision.String()}} {
		created := env.request(t, http.MethodPost, env.proposalsPath(t, w), author.String(), authorProposalBody(asset.id, asset.rev, author.String()))
		if created.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", created.Code, created.Body.String())
		}
		id := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
		submitted := env.request(t, http.MethodPost, env.proposalsPath(t, w)+"/"+id+"/submit", author.String(), "")
		if submitted.Code != http.StatusOK {
			t.Fatalf("submit: %d", submitted.Code)
		}
		env.runValidationWorker(t)
		approveProposal(t, env, w, reviewer, id)
		published := publishProposal(t, env, w, publisher, id)
		if published.Code != http.StatusCreated {
			t.Fatalf("publish: %d %s", published.Code, published.Body.String())
		}
	}
	snapshot, err := env.store.CurrentReleaseSnapshot(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Assets) != 2 {
		t.Fatalf("published dependency carry-forward absent: got %d assets", len(snapshot.Assets))
	}
	for _, a := range snapshot.Assets {
		if a.AssetID == draft {
			t.Fatal("unpublished draft entered release")
		}
	}
}

func TestExecutionPublishedPlanReadsRealPostgres(t *testing.T) {
	proveExecutionPostgresFixture(t, false)
}

func TestExecutionCurrentBindingCancellationAfterPublication(t *testing.T) {
	proveExecutionPostgresFixture(t, true)
}

func proveExecutionPostgresFixture(t *testing.T, channelsOnly bool) {
	env := newFixture(t)
	ctx := context.Background()
	w := createWorkspace(t, env.pool, "execution-real")
	author := createPrincipalWithRoles(t, env, w, "author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, env, w, "publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, env, w, "reviewer")
	executor := createPrincipalWithRoles(t, env, w, "executor", []string{"workspace_admin"})
	asset, revision := env.createAsset(t, w)
	catalog := catalogapp.NewService(env.store, catalogapp.ClockFunc(time.Now))
	appended, err := catalog.AppendRevision(ctx, catalogapp.AppendRevisionRequest{WorkspaceID: w, AssetID: asset, SchemaVersion: "1.0.0", Content: json.RawMessage(`{"name":"Net revenue","definition":"Revenue after refunds","execution":{"aggregation":"sum"}}`), CreatedBy: "founder", TraceID: traceID})
	if err != nil {
		t.Fatal(err)
	}
	revision = appended.ID
	_, _, proposal := proposeToInReview(t, env, w, author, asset, revision)
	approveProposal(t, env, w, reviewer, proposal)
	if response := publishProposal(t, env, w, publisher, proposal); response.Code != http.StatusCreated {
		t.Fatalf("publish asset: %d %s", response.Code, response.Body.String())
	}
	src, _ := identity.NewSourceConnectionID()
	srv, _ := identity.NewSourceRevisionID()
	dataset, _ := identity.NewPhysicalDatasetID()
	dr, _ := identity.NewPhysicalDatasetRevisionID()
	field, _ := identity.NewPhysicalFieldID()
	fr, _ := identity.NewPhysicalFieldRevisionID()
	binding, _ := identity.NewPhysicalBindingID()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	locator := parsed.Host + parsed.Path
	statements := []struct {
		sql  string
		args []any
	}{
		{`CREATE TABLE public.execution_facts(amount numeric NOT NULL)`, nil},
		{`INSERT INTO public.execution_facts VALUES(123456789.123456789),(0.000000001)`, nil},
		{`INSERT INTO source_connections(id,workspace_id,adapter_kind,name,normalized_locator,status) VALUES($1,$2,'postgresql_catalog','Execution source',$3,'active')`, []any{src.UUID(), w.UUID(), locator}},
		{`INSERT INTO source_revisions(id,workspace_id,source_connection_id,content_digest,adapter_version,observed_at) VALUES($1,$2,$3,$4,'test/1',CURRENT_TIMESTAMP)`, []any{srv.UUID(), w.UUID(), src.UUID(), digestOf("source")}},
		{`INSERT INTO physical_datasets(id,workspace_id,source_connection_id,external_key,qualified_name) VALUES($1,$2,$3,'execution_facts','public.execution_facts')`, []any{dataset.UUID(), w.UUID(), src.UUID()}},
		{`INSERT INTO physical_dataset_revisions(id,workspace_id,physical_dataset_id,source_revision_id,dataset_kind,locator,content_digest) VALUES($1,$2,$3,$4,'table','public.execution_facts',$5)`, []any{dr.UUID(), w.UUID(), dataset.UUID(), srv.UUID(), digestOf("dataset")}},
		{`UPDATE physical_datasets SET current_revision_id=$2 WHERE id=$1`, []any{dataset.UUID(), dr.UUID()}},
		{`INSERT INTO physical_fields(id,workspace_id,physical_dataset_id,external_key,name) VALUES($1,$2,$3,'amount','amount')`, []any{field.UUID(), w.UUID(), dataset.UUID()}},
		{`INSERT INTO physical_field_revisions(id,workspace_id,physical_field_id,dataset_revision_id,ordinal,data_type,nullable) VALUES($1,$2,$3,$4,1,'numeric',false)`, []any{fr.UUID(), w.UUID(), field.UUID(), dr.UUID()}},
		{`UPDATE physical_fields SET current_revision_id=$2 WHERE id=$1`, []any{field.UUID(), fr.UUID()}},
		{`INSERT INTO physical_bindings(id,workspace_id,asset_id,dataset_id,field_id,created_by) VALUES($1,$2,$3,$4,$5,'steward')`, []any{binding.UUID(), w.UUID(), asset.UUID(), dataset.UUID(), field.UUID()}},
	}
	for _, s := range statements {
		if _, err := env.pool.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	body := fmt.Sprintf(`{"targetObjectType":"physical_binding","targetObjectId":%q,"title":"Publish execution binding","reason":"Governed execution","createdBy":%q,"changeSet":[{"fieldPath":"notes","op":"add","afterDigest":%q,"afterValue":"Execution binding reviewed"}]}`, binding.String(), author.String(), sha256Of(`"Execution binding reviewed"`))
	created := env.request(t, http.MethodPost, env.proposalsPath(t, w), author.String(), body)
	if created.Code != http.StatusCreated {
		t.Fatalf("binding create: %d %s", created.Code, created.Body.String())
	}
	id := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
	submitted := env.request(t, http.MethodPost, env.proposalsPath(t, w)+"/"+id+"/submit", author.String(), "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("binding submit: %d %s", submitted.Code, submitted.Body.String())
	}
	env.runValidationWorker(t)
	approveProposal(t, env, w, reviewer, id)
	if response := publishProposal(t, env, w, publisher, id); response.Code != http.StatusCreated {
		proposalID, _ := identity.ParseProposalID(id)
		diagnostic := governanceapp.NewPublishingService(env.store, authapp.NewService(env.store, authapp.ClockFunc(time.Now)), governanceapp.ClockFunc(time.Now))
		_, cause := diagnostic.PublishProposal(ctx, governanceapp.PublishProposalRequest{WorkspaceID: w, ProposalID: proposalID, PrincipalRef: publisher.String(), TraceID: traceID})
		// Repository errors expose only the operation and stable domain code.
		t.Logf("failed binding publication diagnostic retry (original failure retained): %v", cause)
		t.Fatalf("publish binding: %d %s", response.Code, response.Body.String())
	}
	auth := authapp.NewService(env.store, authapp.ClockFunc(time.Now))
	plans := distapp.NewService(env.store, auth, distapp.ClockFunc(time.Now))
	resolved, err := plans.Resolve(ctx, distapp.ResolveRequest{WorkspaceID: w, PrincipalRef: executor.String(), TraceID: traceID, Channel: "api", IdempotencyKey: "published-plan", Input: dist.SemanticQueryInput{SchemaVersion: "1.0.0", Intent: dist.IntentAggregate, Measures: []dist.Selector{{AssetID: &asset}}, Filters: []dist.Filter{{Selector: dist.Selector{AssetID: &asset}, Operator: "gte", Value: json.RawMessage(`123456789.123456789`)}}, Context: dist.ResolutionContext{Mode: dist.ResolutionCurrent}}})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Plan == nil || resolved.Plan.Execution == nil {
		t.Fatalf("normal publication has no executable plan: %#v", resolved.Refusal)
	}
	if _, err := env.pool.Exec(ctx, `CREATE ROLE execution_reader LOGIN PASSWORD 'fixture-only'; REVOKE TEMP ON DATABASE semlia_governance_http_test FROM PUBLIC; GRANT USAGE ON SCHEMA public TO execution_reader; GRANT SELECT ON public.execution_facts TO execution_reader; ALTER ROLE execution_reader SET log_min_error_statement='panic'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{`DROP TABLE IF EXISTS public.execution_facts`, `DROP OWNED BY execution_reader`, `DROP ROLE execution_reader`} {
			if _, err := env.pool.Exec(context.Background(), query); err != nil {
				t.Error("fixture cleanup failed")
			}
		}
	})
	parsed.User = url.UserPassword("execution_reader", "fixture-only")
	config, _ := json.Marshal([]execadapter.SourceReference{{WorkspaceID: w.String(), SourceID: src.String(), DSNEnv: "SEMLIA_EXECUTION_DSN_TEST"}})
	adapter, err := execadapter.NewPostgresWithLookup(string(config), func(string) (string, bool) { return parsed.String(), true })
	if err != nil {
		t.Fatal(err)
	}
	adapter.WithLocalPlaintext()
	compiled, err := execdomain.Compile(*resolved.Plan)
	if err != nil {
		t.Fatal(err)
	}
	bound := compiled
	bound.SQL = `SELECT $1::numeric(18,2),$2::date,$3::uuid,$4::timestamptz,$5::varchar(64),current_setting('transaction_read_only') FROM public.execution_facts LIMIT 1`
	bound.Args = []any{"9007199254740993.12", "2026-09-05", src.UUID(), "2026-09-05T11:00:00Z", "precision"}
	bound.Columns = []string{"amount", "date", "uuid", "time", "label", "readOnly"}
	boundOut := adapter.Execute(ctx, w, bound, execdomain.DefaultLimits())
	if len(boundOut.Rows) == 1 && boundOut.Rows[0][2] != src.UUID() {
		t.Fatal("UUID result must be a canonical string")
	}
	if boundOut.ErrorCode != "" || len(boundOut.Rows) != 1 || fmt.Sprint(boundOut.Rows[0][0]) != "9007199254740993.12" || boundOut.Rows[0][5] != "on" {
		conn, e := pgx.Connect(ctx, parsed.String())
		if e == nil {
			defer conn.Close(ctx)
			var safe bool
			e = conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles r WHERE pg_catalog.pg_has_role(current_user,r.oid,'MEMBER') AND (pg_catalog.has_database_privilege(r.oid,current_database(),'CREATE,TEMP') OR EXISTS(SELECT 1 FROM pg_catalog.pg_namespace n WHERE pg_catalog.has_schema_privilege(r.oid,n.oid,'CREATE')) OR EXISTS(SELECT 1 FROM pg_catalog.pg_class c WHERE c.relkind='S' AND pg_catalog.has_sequence_privilege(r.oid,c.oid,'USAGE,UPDATE')) OR EXISTS(SELECT 1 FROM pg_catalog.pg_class c WHERE c.relkind IN('r','p','v','m','f') AND c.oid<>'pg_catalog.pg_settings'::regclass AND (pg_catalog.has_table_privilege(r.oid,c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER') OR pg_catalog.has_any_column_privilege(r.oid,c.oid,'INSERT,UPDATE,REFERENCES')))))`).Scan(&safe)
			t.Logf("static role policy probe: unsafe=%v diagnostic=%v", safe, e)
		}
		t.Fatalf("typed extended params/read-only transaction failed: %s", boundOut.ErrorCode)
	}
	rowQuery := compiled
	rowQuery.SQL = `SELECT amount FROM public.execution_facts`
	rowQuery.Args = nil
	rowLimits := execdomain.DefaultLimits()
	rowLimits.MaxRows = 1
	if out := adapter.Execute(ctx, w, rowQuery, rowLimits); out.ErrorCode != "EXECUTION_ROW_LIMIT" || len(out.Rows) != 0 {
		t.Fatal("actual row cap failed")
	}
	queryParams := parsed.Query()
	queryParams.Set("default_query_exec_mode", "simple_protocol")
	parsed.RawQuery = queryParams.Encode()
	simpleOverride, _ := execadapter.NewPostgresWithLookup(string(config), func(string) (string, bool) { return parsed.String(), true })
	simpleOverride.WithLocalPlaintext()
	protocolProbe := compiled
	protocolProbe.SQL = `SELECT $1::text; SELECT $1::text`
	protocolProbe.Args = []any{"bound-only-marker"}
	if out := simpleOverride.Execute(ctx, w, protocolProbe, execdomain.DefaultLimits()); out.ErrorCode != "EXECUTION_SOURCE_FAILED" {
		t.Fatal("DSN simple protocol override enabled interpolation")
	}
	spy := &executionSpy{adapter: adapter}
	service := execapp.NewService(env.store, plans, auth, spy, execdomain.DefaultLimits())
	if channelsOnly {
		var input dist.SemanticQueryInput
		if json.Unmarshal(resolved.Query.CanonicalRequest, &input) != nil {
			t.Fatal("canonical query")
		}
		proveExecutionChannels(t, env, w, executor, plans, service, spy, input, func() {
			extra, revision := env.createAssetWithAddress(t, w, "execution.next_release")
			_, _, proposal := proposeToInReview(t, env, w, author, extra, revision)
			approveProposal(t, env, w, reviewer, proposal)
			if response := publishProposal(t, env, w, publisher, proposal); response.Code != http.StatusCreated {
				t.Fatalf("concurrent release publication failed: %d", response.Code)
			}
		})
		return
	}
	request := execapp.Request{WorkspaceID: w, PlanID: resolved.Plan.ID, PlanDigest: resolved.Plan.PlanDigest, IdempotencyKey: "real-execution", Channel: "api", PrincipalRef: executor.String(), TraceID: traceID}
	result, err := service.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.State != "succeeded" || len(result.Rows) != 1 || fmt.Sprint(result.Rows[0][0]) != "123456789.123456790" {
		conn, connectErr := pgx.Connect(ctx, parsed.String())
		if connectErr == nil {
			defer conn.Close(ctx)
			for _, setting := range []string{"log_statement", "log_min_duration_statement", "log_min_duration_sample", "log_parameter_max_length_on_error", "log_min_error_statement"} {
				var value string
				settingErr := conn.QueryRow(ctx, `SELECT current_setting($1)`, setting).Scan(&value)
				t.Logf("read-only policy %s=%s error=%v", setting, value, settingErr)
			}
			for _, query := range []string{`SELECT nspname FROM pg_namespace WHERE has_schema_privilege(current_user,oid,'CREATE')`, `SELECT relname FROM pg_class WHERE relkind IN ('r','p','v','m','f') AND (has_table_privilege(current_user,oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER') OR has_any_column_privilege(current_user,oid,'INSERT,UPDATE,REFERENCES'))`, `SELECT rolname FROM pg_roles WHERE pg_has_role(current_user,oid,'MEMBER') AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)`, `SELECT current_setting('transaction_read_only'),has_database_privilege(current_user,current_database(),'TEMP')::text`} {
				rows, e := conn.Query(ctx, query)
				if e != nil {
					t.Log("role policy diagnostic", e)
					continue
				}
				for rows.Next() {
					v, _ := rows.Values()
					t.Log("role policy diagnostic", v)
				}
				t.Log("role policy query error", rows.Err())
				rows.Close()
			}
		}
		t.Fatalf("real execution failed: state=%s code=%s rows=%v", result.Run.State, result.Run.ErrorCode, result.Rows)
	}
	replay, err := service.Execute(ctx, request)
	if err != nil || !replay.Replay || replay.Availability != "metadata_only" || len(replay.Rows) != 0 || replay.Run.ID != result.Run.ID {
		t.Fatalf("metadata replay invalid: %#v %v", replay, err)
	}
	if spy.calls.Load() != 1 {
		t.Fatal("replay reexecuted source")
	}
	concurrent := request
	concurrent.IdempotencyKey = "concurrent"
	var wg sync.WaitGroup
	outcomes := make(chan execdomain.Result, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := service.Execute(ctx, concurrent)
			if err != nil {
				failures <- err
			} else {
				outcomes <- out
			}
		}()
	}
	wg.Wait()
	close(outcomes)
	close(failures)
	for err := range failures {
		t.Fatal("concurrent execution:", err)
	}
	ids := map[string]bool{}
	for out := range outcomes {
		ids[out.Run.ID] = true
	}
	if len(ids) != 1 || spy.calls.Load() != 2 {
		t.Fatalf("duplicate execution: runs=%d calls=%d", len(ids), spy.calls.Load())
	}
	// A durable claim left by a lost process cannot be reclaimed by a new service.
	orphan := result.Run
	orphanID, _ := identity.NewRunID()
	orphan.ID = orphanID.String()
	orphan.IdempotencyKey = "lost-process"
	orphan.State = "running"
	orphan.ResultDigest = ""
	orphan.RowCount = 0
	orphan.ByteCount = 0
	orphan.FinishedAt = nil
	orphan.StartedAt = time.Now().Add(-time.Minute)
	orphan.Deadline = time.Now().Add(-time.Second)
	var version int64
	if err := env.pool.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1`, w.UUID()).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if _, owner, err := env.store.ClaimExecution(ctx, orphan, 2, version); err != nil || !owner {
		t.Fatalf("orphan claim: %v", err)
	}
	restarted := execapp.NewService(env.store, plans, auth, spy, execdomain.DefaultLimits())
	lostRequest := request
	lostRequest.IdempotencyKey = "lost-process"
	lost, err := restarted.Execute(ctx, lostRequest)
	if err != nil || lost.Run.State != "unknown" || !lost.Replay || spy.calls.Load() != 2 {
		t.Fatalf("lost process reexecution: state=%s err=%v", lost.Run.State, err)
	}
	beforeCalls := spy.calls.Load()
	tampered := request
	tampered.PlanDigest = "sha256:" + strings.Repeat("0", 64)
	if _, err := service.Execute(ctx, tampered); err == nil || spy.calls.Load() != beforeCalls {
		t.Fatal("tampered plan reached source")
	}
	other := createWorkspace(t, env.pool, "execution-other")
	cross := request
	cross.WorkspaceID = other
	if _, err := service.Execute(ctx, cross); err == nil || spy.calls.Load() != beforeCalls {
		t.Fatal("cross workspace reached source")
	}
	if _, err := env.pool.Exec(ctx, `UPDATE source_connections SET status='paused' WHERE id=$1`, src.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Execute(ctx, request); err == nil || spy.calls.Load() != beforeCalls {
		t.Fatal("paused source bypassed on replay")
	}
	if _, err := env.pool.Exec(ctx, `UPDATE source_connections SET status='active' WHERE id=$1`, src.UUID()); err != nil {
		t.Fatal(err)
	}
	// The dedicated credential must lack table and column DML privileges.
	for _, grant := range []string{"INSERT", "UPDATE(amount)", "DELETE", "TRUNCATE"} {
		if _, err := env.pool.Exec(ctx, "GRANT "+grant+" ON public.execution_facts TO execution_reader"); err != nil {
			t.Fatal(err)
		}
		unsafeRequest := request
		unsafeRequest.IdempotencyKey = "unsafe-role-" + strings.ToLower(strings.ReplaceAll(grant, "(amount)", "-column"))
		out, err := service.Execute(ctx, unsafeRequest)
		if err != nil || out.Run.ErrorCode != "EXECUTION_ROLE_NOT_READ_ONLY" {
			t.Fatalf("unsafe role %s accepted: %s %v", grant, out.Run.ErrorCode, err)
		}
		if _, err := env.pool.Exec(ctx, "REVOKE "+grant+" ON public.execution_facts FROM execution_reader"); err != nil {
			t.Fatal(err)
		}
	}
	limits := execdomain.DefaultLimits()
	if _, err := env.pool.Exec(ctx, `CREATE ROLE execution_writer;GRANT INSERT ON public.execution_facts TO execution_writer;GRANT execution_writer TO execution_reader WITH INHERIT FALSE`); err != nil {
		t.Fatal(err)
	}
	memberRequest := request
	memberRequest.IdempotencyKey = "noinherit-writer"
	if out, err := service.Execute(ctx, memberRequest); err != nil || out.Run.ErrorCode != "EXECUTION_ROLE_NOT_READ_ONLY" {
		t.Fatalf("reachable writer role accepted: %s %v", out.Run.ErrorCode, err)
	}
	if _, err := env.pool.Exec(ctx, `REVOKE execution_writer FROM execution_reader;DROP OWNED BY execution_writer;DROP ROLE execution_writer`); err != nil {
		t.Fatal(err)
	}
	limits.MaxBytes = 16
	small := execapp.NewService(env.store, plans, auth, spy, limits)
	smallRequest := request
	smallRequest.IdempotencyKey = "byte-cap"
	bounded, err := small.Execute(ctx, smallRequest)
	if err != nil || bounded.Run.ErrorCode != "EXECUTION_BYTE_LIMIT" || len(bounded.Rows) != 0 {
		t.Fatalf("byte cap invalid: %s %v", bounded.Run.ErrorCode, err)
	}
	lock, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err := lock.Exec(ctx, `LOCK TABLE public.execution_facts IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	limits = execdomain.DefaultLimits()
	timedService := execapp.NewService(env.store, plans, auth, spy, limits)
	timedRequest := request
	timedRequest.IdempotencyKey = "timeout"
	timedResult := make(chan execdomain.Result, 1)
	timedError := make(chan error, 1)
	go func() {
		result, err := timedService.Execute(ctx, timedRequest)
		timedResult <- result
		timedError <- err
	}()
	observedTimeoutLock := false
	for until := time.Now().Add(limits.Timeout - time.Second); time.Now().Before(until); {
		if err := env.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name='semlia-execution' AND wait_event_type='Lock')`).Scan(&observedTimeoutLock); err != nil {
			t.Fatal(err)
		}
		if observedTimeoutLock {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	timed, err := <-timedResult, <-timedError
	if !observedTimeoutLock || err != nil || timed.Run.ErrorCode != "EXECUTION_TIMEOUT" || len(timed.Rows) != 0 {
		t.Fatalf("timeout invalid: %s %v", timed.Run.ErrorCode, err)
	}
	durableRequest := request
	durableRequest.IdempotencyKey = "durable-cancel"
	cancelOutcome := make(chan execdomain.Result, 1)
	cancelError := make(chan error, 1)
	go func() { out, err := service.Execute(ctx, durableRequest); cancelOutcome <- out; cancelError <- err }()
	var durableID string
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		var raw string
		if env.pool.QueryRow(ctx, `SELECT id::text FROM query_execution_runs WHERE workspace_id=$1 AND idempotency_key='durable-cancel'`, w.UUID()).Scan(&raw) == nil {
			id, _ := identity.FromUUID(identity.Run, raw)
			durableID = id.String()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if durableID == "" {
		t.Fatal("durable cancellation claim missing")
	}
	active := false
	until = time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		if err := env.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name='semlia-execution' AND wait_event_type='Lock')`).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if active {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !active {
		t.Fatal("durable cancellation source query never became active")
	}
	newRevision, _ := identity.NewPhysicalDatasetRevisionID()
	advancedSource, _ := identity.NewSourceRevisionID()
	if _, err := env.pool.Exec(ctx, `INSERT INTO source_revisions(id,workspace_id,source_connection_id,content_digest,adapter_version,observed_at) VALUES($1,$2,$3,$4,'test/2',CURRENT_TIMESTAMP)`, advancedSource.UUID(), w.UUID(), src.UUID(), digestOf("advanced-source")); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO physical_dataset_revisions(id,workspace_id,physical_dataset_id,source_revision_id,dataset_kind,locator,content_digest) VALUES($1,$2,$3,$4,'table','public.execution_facts',$5)`, newRevision.UUID(), w.UUID(), dataset.UUID(), advancedSource.UUID(), digestOf("advanced-dataset")); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `UPDATE physical_datasets SET current_revision_id=$2 WHERE id=$1`, dataset.UUID(), newRevision.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, w, durableID, executor.String(), traceID); err == nil {
		t.Fatal("freshness check unexpectedly accepts advanced source revision")
	}
	requested, err := service.Cancel(ctx, w, durableID, executor.String(), traceID)
	if err != nil || !requested.Run.CancelRequested || (requested.Run.State != "running" && requested.Run.State != "cancelled") {
		t.Fatalf("cancel request invalid: state=%s requested=%t error=%v", requested.Run.State, requested.Run.CancelRequested, err)
	}
	select {
	case out := <-cancelOutcome:
		if err := <-cancelError; err != nil || out.Run.State != "cancelled" {
			t.Fatalf("durable cancel failed: %s %v", out.Run.State, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("durable cancellation did not stop source query")
	}
	if _, err := env.pool.Exec(ctx, `UPDATE physical_datasets SET current_revision_id=$2 WHERE id=$1`, dataset.UUID(), dr.UUID()); err != nil {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cancelRequest := request
	cancelRequest.IdempotencyKey = "cancel"
	callerResult := make(chan execdomain.Result, 1)
	callerError := make(chan error, 1)
	go func() {
		result, err := service.Execute(cancelCtx, cancelRequest)
		callerResult <- result
		callerError <- err
	}()
	callerActive := false
	for until := time.Now().Add(5 * time.Second); time.Now().Before(until); {
		if err := env.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name='semlia-execution' AND wait_event_type='Lock')`).Scan(&callerActive); err != nil {
			t.Fatal(err)
		}
		if callerActive {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	cancelled, err := <-callerResult, <-callerError
	if !callerActive || err != nil || cancelled.Run.ErrorCode != "EXECUTION_CANCELLED" || len(cancelled.Rows) != 0 {
		t.Fatalf("cancel invalid: %s %v", cancelled.Run.ErrorCode, err)
	}
	if err := lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	raceRun := result.Run
	raceID, _ := identity.NewRunID()
	raceRun.ID = raceID.String()
	raceRun.IdempotencyKey = "cancel-completion-race"
	raceRun.State = "running"
	raceRun.ResultDigest = ""
	raceRun.FinishedAt = nil
	raceRun.Deadline = time.Now().Add(time.Minute)
	if err := env.pool.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1`, w.UUID()).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if _, owner, err := env.store.ClaimExecution(ctx, raceRun, 2, version); err != nil || !owner {
		t.Fatal("race claim", err)
	}
	raceRepo := &cancelCompletionRepository{Store: env.store, complete: func() {
		finished := time.Now()
		raceRun.State = "succeeded"
		raceRun.FinishedAt = &finished
		raceRun.ResultDigest = result.Run.ResultDigest
		if err := env.store.FinishExecution(ctx, raceRun); err != nil {
			t.Fatal(err)
		}
	}}
	raceService := execapp.NewService(raceRepo, plans, auth, spy, execdomain.DefaultLimits())
	raceResult, err := raceService.Cancel(ctx, w, raceRun.ID, executor.String(), traceID)
	if err != nil || raceResult.Run.State != "succeeded" || raceResult.Run.CancelRequested {
		t.Fatalf("cancel invented accepted request after concurrent finish: %+v %v", raceResult.Run, err)
	}
	crashLock, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer crashLock.Rollback(ctx)
	if _, err := crashLock.Exec(ctx, `LOCK TABLE public.execution_facts IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	crashRequest := request
	crashRequest.IdempotencyKey = "actual-process-loss"
	requestJSON, _ := json.Marshal(crashRequest)
	child := exec.Command(binary, "-test.run=^TestExecutionCrashWorker$")
	child.Env = append(os.Environ(), "SEMLIA_EXECUTION_CRASH_HELPER=1", "SEMLIA_EXECUTION_TEST_DB="+databaseURL, "SEMLIA_EXECUTION_TEST_CONFIG="+string(config), "SEMLIA_EXECUTION_DSN_TEST="+parsed.String(), "SEMLIA_EXECUTION_TEST_REQUEST="+string(requestJSON), "SEMLIA_EXECUTION_TEST_WORKSPACE="+w.String(), "SEMLIA_EXECUTION_TEST_PRINCIPAL="+executor.String())
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill() })
	childDone := make(chan error, 1)
	go func() { childDone <- child.Wait() }()
	observed := false
	until = time.Now().Add(execdomain.DefaultLimits().Timeout)
	for time.Now().Before(until) {
		select {
		case err := <-childDone:
			t.Fatalf("crash helper exited before source SELECT: %v", err)
		default:
		}
		if err := env.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name='semlia-execution' AND wait_event_type='Lock')`).Scan(&observed); err != nil {
			t.Fatal(err)
		}
		if observed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !observed {
		t.Fatal("child did not reach real source SELECT")
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = <-childDone
	if err := crashLock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var claimedDeadline time.Time
	if err := env.pool.QueryRow(ctx, `SELECT deadline_at FROM query_execution_runs WHERE workspace_id=$1 AND idempotency_key='actual-process-loss'`, w.UUID()).Scan(&claimedDeadline); err != nil {
		t.Fatal(err)
	}
	if remaining := time.Until(claimedDeadline) + 100*time.Millisecond; remaining > 0 {
		time.Sleep(remaining)
	}
	beforeCrashReplay := spy.calls.Load()
	crashReplay, err := service.Execute(ctx, crashRequest)
	if err != nil || crashReplay.Run.State != "unknown" || !crashReplay.Replay || len(crashReplay.Rows) != 0 || spy.calls.Load() != beforeCrashReplay {
		t.Fatalf("actual process loss reexecuted or invented outcome: %s %v", crashReplay.Run.State, err)
	}
	var machineInput dist.SemanticQueryInput
	if json.Unmarshal(resolved.Query.CanonicalRequest, &machineInput) != nil {
		t.Fatal("canonical query")
	}
	proveExecutionChannels(t, env, w, executor, plans, service, spy, machineInput, func() {
		extra, revision := env.createAssetWithAddress(t, w, "execution.next_release")
		_, _, proposal := proposeToInReview(t, env, w, author, extra, revision)
		approveProposal(t, env, w, reviewer, proposal)
		if response := publishProposal(t, env, w, publisher, proposal); response.Code != http.StatusCreated {
			t.Fatalf("concurrent release publication failed: %d", response.Code)
		}
	})
	fresh, err := plans.Resolve(ctx, distapp.ResolveRequest{WorkspaceID: w, PrincipalRef: executor.String(), TraceID: traceID, Channel: "api", IdempotencyKey: "after-machine-current-publication", Input: machineInput})
	if err != nil || fresh.Plan == nil {
		t.Fatal("fresh current plan", err)
	}
	request.PlanID, request.PlanDigest = fresh.Plan.ID, fresh.Plan.PlanDigest
	if os.Getenv("SEMLIA_EXECUTION_BROWSER_PROOF") == "1" {
		proveLiveAskExecution(t, env, w, executor, asset, plans, service)
	}
	settings, err := env.store.GetRuntimeSettings(ctx, w, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	settings.QueryRowLimit, settings.QueryByteLimit, settings.StatementTimeoutMS = 1, 1024, 1000
	settings, err = env.store.UpdateRuntimeSettings(ctx, settings, settings.Version)
	if err != nil {
		t.Fatal(err)
	}
	policyRequest := request
	policyRequest.IdempotencyKey = "current-policy"
	policyResult, err := service.Execute(ctx, policyRequest)
	if err != nil || policyResult.Run.State != "succeeded" || policyResult.Run.MaxRows != 1 || policyResult.Run.MaxBytes != 1024 || policyResult.Run.TimeoutMS != 1000 || policyResult.Run.PolicyVersion != settings.Version {
		t.Fatalf("runtime policy ignored: %+v %v", policyResult.Run, err)
	}
	if !strings.HasPrefix(policyResult.Run.SourceID, "src_") || !strings.HasPrefix(policyResult.Run.SourceRevisionID, "srv_") {
		t.Fatal("public execution leaked raw UUID")
	}
	var persisted string
	if err := env.pool.QueryRow(ctx, `SELECT COALESCE((SELECT jsonb_agg(to_jsonb(r))::text FROM query_execution_runs r WHERE workspace_id=$1),'') || COALESCE((SELECT jsonb_agg(to_jsonb(r))::text FROM runtime_runs r WHERE workspace_id=$1),'') || COALESCE((SELECT jsonb_agg(to_jsonb(a))::text FROM audit_events a WHERE workspace_id=$1),'') || COALESCE((SELECT jsonb_agg(to_jsonb(u))::text FROM usage_events u WHERE workspace_id=$1),'') || COALESCE((SELECT jsonb_agg(to_jsonb(o))::text FROM outbox_events o WHERE workspace_id=$1),'')`, w.UUID()).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"123456789.123456790", "123456789.123456789", "fixture-only", "SELECT SUM"} {
		if strings.Contains(persisted, secret) {
			t.Fatal("execution persistence contains sensitive result, credential or SQL")
		}
	}
	// Carry-forward copies the published version even when its mutable object
	// has an unrelated draft; a different binding may not silently replace it.
	if _, err := env.pool.Exec(ctx, `UPDATE physical_bindings SET version=version+1,content=jsonb_set(content,'{notes}','"unpublished binding draft"') WHERE id=$1`, binding.UUID()); err != nil {
		t.Fatal(err)
	}
	publishExtra := func(address string, changePhysical bool) {
		t.Helper()
		extra, revision := env.createAssetWithAddress(t, w, address)
		_, _, proposal := proposeToInReview(t, env, w, author, extra, revision)
		approveProposal(t, env, w, reviewer, proposal)
		if response := publishProposal(t, env, w, publisher, proposal); response.Code != 201 {
			t.Fatalf("carry draft publication: %d", response.Code)
		}
		if changePhysical {
			if _, err := env.pool.Exec(ctx, `UPDATE physical_fields SET name='changed_amount' WHERE id=$1`, field.UUID()); err != nil {
				t.Fatal(err)
			}
		}
		extraBinding, _ := identity.NewPhysicalBindingID()
		if _, err := env.pool.Exec(ctx, `INSERT INTO physical_bindings(id,workspace_id,asset_id,dataset_id,field_id,created_by) VALUES($1,$2,$3,$4,$5,'steward')`, extraBinding.UUID(), w.UUID(), extra.UUID(), dataset.UUID(), field.UUID()); err != nil {
			t.Fatal(err)
		}
		created := env.request(t, http.MethodPost, env.proposalsPath(t, w), author.String(), strings.Replace(body, binding.String(), extraBinding.String(), 1))
		if created.Code != 201 {
			t.Fatalf("extra binding create: %d", created.Code)
		}
		id := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
		if response := env.request(t, http.MethodPost, env.proposalsPath(t, w)+"/"+id+"/submit", author.String(), ""); response.Code != 200 {
			t.Fatal("extra binding submit")
		}
		env.runValidationWorker(t)
		approveProposal(t, env, w, reviewer, id)
		var before int
		if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM releases WHERE workspace_id=$1`, w.UUID()).Scan(&before); err != nil {
			t.Fatal(err)
		}
		response := publishProposal(t, env, w, publisher, id)
		if !changePhysical && response.Code != 201 {
			t.Fatalf("equal physical projections refused: %d %s", response.Code, response.Body.String())
		}
		if changePhysical {
			if response.Code != 422 && response.Code != 409 {
				t.Fatalf("conflicting physical projections accepted: %d", response.Code)
			}
			var after, version int
			if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM releases WHERE workspace_id=$1`, w.UUID()).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if err := env.pool.QueryRow(ctx, `SELECT version FROM physical_bindings WHERE id=$1`, extraBinding.UUID()).Scan(&version); err != nil {
				t.Fatal(err)
			}
			if before != after || version != 1 {
				t.Fatal("conflicting publication was not atomic")
			}
		}
		snapshot, err := env.store.CurrentReleaseSnapshot(ctx, w)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Execution == nil || snapshot.Execution.Relations[0].Fields[0].Name != "amount" {
			t.Fatal("carry-forward substituted mutable physical field")
		}
		var currentVersion int
		if err := env.pool.QueryRow(ctx, `SELECT version FROM physical_bindings WHERE id=$1`, binding.UUID()).Scan(&currentVersion); err != nil || currentVersion != 3 {
			t.Fatal("carry-forward overwrote unrelated unpublished draft")
		}
	}
	publishExtra("finance.same_projection", false)
	publishExtra("finance.conflicting_projection", true)
	t.Log("real governed publication, parameterized read-only PostgreSQL aggregate and metadata-only replay passed")
}

type executionSpy struct {
	adapter execapp.Adapter
	calls   atomic.Int64
}

type cancelCompletionRepository struct {
	*pgstore.Store
	complete func()
}

func (r *cancelCompletionRepository) RequestExecutionCancellation(ctx context.Context, w identity.WorkspaceID, id string) error {
	r.complete()
	return r.Store.RequestExecutionCancellation(ctx, w, id)
}

func TestExecutionCrashWorker(t *testing.T) {
	if os.Getenv("SEMLIA_EXECUTION_CRASH_HELPER") != "1" {
		t.Skip("subprocess-only proof")
	}
	ctx := context.Background()
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal("open helper database")
	}
	defer pool.Close()
	store := pgstore.NewStore(pool)
	auth := authapp.NewService(store, authapp.ClockFunc(time.Now))
	plans := distapp.NewService(store, auth, distapp.ClockFunc(time.Now))
	adapter, err := execadapter.NewPostgres(os.Getenv("SEMLIA_EXECUTION_TEST_CONFIG"))
	if err != nil {
		t.Fatal("helper adapter config")
	}
	adapter.WithLocalPlaintext()
	limits := execdomain.DefaultLimits()
	service := execapp.NewService(store, plans, auth, adapter, limits)
	var request execapp.Request
	if json.Unmarshal([]byte(os.Getenv("SEMLIA_EXECUTION_TEST_REQUEST")), &request) != nil {
		t.Fatal("helper request")
	}
	request.WorkspaceID, _ = identity.ParseWorkspaceID(os.Getenv("SEMLIA_EXECUTION_TEST_WORKSPACE"))
	request.PrincipalRef = os.Getenv("SEMLIA_EXECUTION_TEST_PRINCIPAL")
	request.TraceID = traceID
	_, _ = service.Execute(ctx, request)
}

func (s *executionSpy) Execute(ctx context.Context, w identity.WorkspaceID, q execdomain.Compiled, l execdomain.Limits) execdomain.Output {
	s.calls.Add(1)
	return s.adapter.Execute(ctx, w, q, l)
}
