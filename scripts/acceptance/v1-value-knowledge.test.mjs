import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtempSync, readFileSync, lstatSync, rmSync, realpathSync, chmodSync, renameSync, symlinkSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { checkpoint, openState, writePrivate, digest, fixtureDigest } from './v1/core.mjs';
import { supplementContent, valueGlossarySuffix, definitionOnlyTarget, prepareValues, reconcileValues, approvedKnowledgeBasis, sourceBindingProof, validateSourceCategories, readSourceCategories, categorySQL, verifySupplementReceipt, verifySupplementFacts } from './v1/value-knowledge.mjs';
import { governanceHeadProof } from './v1/correction.mjs';

const hash = value => digest(value);
function fixture() {
  const parent = mkdtempSync(join(realpathSync(tmpdir()), 'semlia-values-')), root = join(parent, 'run'), owned = openState(root, hash('owned'));
  const ref = (assetId, revisionId, memberId) => ({ assetId, revisionId, releaseId: 'rls_initial', ...(memberId ? { memberId } : {}) });
  const semantic = ref('ast_customers','rev_customers','region'), dataRef=ref('ast_data','rev_data','region');
  const dataset={snapshotId:'snp_source',kind:'dataset',objectId:'ds_customers',revisionId:'dsrev_customers'}, field={snapshotId:'snp_source',kind:'field',objectId:'fld_region',revisionId:'fldrev_region'};
  const original={assetType:'analysis_model',definition:'Synthetic governed model.',scope:'Synthetic only.',spec:{publicAttributeRefs:[semantic],dataAssetRefs:[ref('ast_data','rev_data')],memberBindings:[{semanticRef:semantic,dataRef}],unchangedRule:{left:'fixed',right:'fixed'}}};
  const savedFixture={DataSQL:'CREATE TABLE synthetic_source_fixture;',Snapshot:{Assets:[{Address:'demo_202609.model',Content:original}]}};
  const input={snapshots:[{sourceId:'src_synthetic',snapshotId:'snp_source',digest:hash('snapshot'),coverageKeys:['customers']}],candidates:[],evidence:[],dependencies:[{kind:'semantic_asset',targetId:'ast_data',revisionId:'rev_data',releaseId:'rls_initial'}]};
  const initialTarget={intent:'create',kind:'semantic_asset',localKey:'model',content:original};
  const state=checkpoint(root,owned,{workspace:{id:'wsp_synthetic'},database:{container:'synthetic_container',port:55432},sourceId:'src_synthetic',fixtureDigest:fixtureDigest(savedFixture),sourceDataDigest:digest(savedFixture.DataSQL),latestRelease:'rls_initial',principals:{author:'prn_author',reviewer:'prn_reviewer',publisher:'prn_publisher'},published:{'demo_202609.model':{kind:'semantic_asset',targetId:'ast_model',revisionId:'rev_original',releaseId:'rls_initial'},'demo_202609.customers':{kind:'semantic_asset',targetId:'ast_customers',revisionId:'rev_customers',releaseId:'rls_initial'},'demo_202609.customer_data':{kind:'semantic_asset',targetId:'ast_data',revisionId:'rev_data',releaseId:'rls_initial'},customers_binding:{kind:'physical_binding',targetId:'bind_customers',objectVersion:1,contentDigest:hash('binding'),releaseId:'rls_binding'},join:{kind:'join_contract',targetId:'join_fixed',objectVersion:2,contentDigest:hash('join'),releaseId:'rls_join'}},operations:{'demo_202609.model':{operationId:'prodop_original',payload:{input,targets:[initialTarget]}},customers_binding:{operationId:'prodop_binding'}},discovery:{snapshot:{id:'snp_source',sourceId:'src_synthetic',contentDigest:hash('snapshot')},members:[{kind:'dataset',name:'semlia_demo_202609.customers',objectId:dataset.objectId,revisionId:dataset.revisionId},{kind:'field',name:'region',parentObjectId:dataset.objectId,objectId:field.objectId,revisionId:field.revisionId}]}});
  writePrivate(join(root,'fixture.json'),savedFixture,true); writePrivate(join(root,'secrets.json'),{readerPassword:'f'.repeat(64)},true);
  for(const name of ['final-candidate.json','model-acceptance.json','correction-amount-acceptance.json'])writePrivate(join(root,name),{owner:state.owner,preserved:name},true);
  const originalOperation={summary:{id:'prodop_original',releaseId:'rls_initial'},input,targets:[{targetId:'ast_model',contentDigest:hash('original'),declaration:initialTarget}]};
  const data={assetId:'ast_data',id:'rev_data',content:{spec:{datasetRef:dataset,members:[{id:'region',sourceFieldRef:field}]}}};
  const connection={id:'src_synthetic',host:'127.0.0.1',port:55432,database:state.owner+'_source',username:state.owner+'_reader',sslMode:'disable'};
  const bindingOperation={summary:{id:'prodop_binding',releaseId:'rls_binding'},targets:[{targetId:'bind_customers',contentDigest:hash('binding'),declaration:{content:{asset:{kind:'semantic_asset',targetId:'ast_data',revisionId:'rev_data',releaseId:'rls_initial'},dataset}}}]};
  bindingOperation.summary.progress='released';bindingOperation.version=1;bindingOperation.setDigest=hash('binding-set');
  Object.assign(bindingOperation.targets[0],{outcome:'proposal',proposalState:'released'});
  Object.assign(bindingOperation.targets[0].declaration,{intent:'create',kind:'physical_binding',localKey:'customers_binding',identityKey:'v1.customers_binding',title:'Synthetic source binding',changes:[],evidenceIds:[]});
  state.operations.customers_binding.payload={targets:[structuredClone(bindingOperation.targets[0].declaration)]};
  delete state.published.customers_binding.contentDigest;delete state.published.join.contentDigest;writePrivate(join(root,'state.json'),state);
  const before={id:'rls_current',afterManifest:{digest:hash('before'),assets:[{assetId:'ast_model',revisionId:'rev_original',position:0,compatibility:{}},{assetId:'ast_customers',revisionId:'rev_customers',position:1,compatibility:{}},{assetId:'ast_data',revisionId:'rev_data',position:2,compatibility:{}}],objects:[{kind:'physical_binding',targetId:'bind_customers',objectVersion:1,contentDigest:hash('binding'),position:0},{kind:'join_contract',targetId:'join_fixed',objectVersion:2,contentDigest:hash('join'),position:1}]}};
  for(const object of before.afterManifest.objects)delete object.contentDigest;
  const originalRelease={...structuredClone(before),id:'rls_initial'};
  const bindingRelease={id:'rls_binding',attribution:{role:'applied',operationId:'prodop_binding',version:1,setDigest:hash('binding-set')},afterManifest:{digest:hash('binding-manifest'),objects:[structuredClone(before.afterManifest.objects[0])]}};
  const baseline={assetId:'ast_model',id:'rev_original',content:original,contentDigest:hash('original')};
  let release=structuredClone(before),current={...baseline},operation,event,reviews=[];
  const calls=[],setDigest=hash('set'),validation={status:'succeeded',attemptNo:1,validationDigest:hash('validation')};
  const command=()=>({operationId:'prodop_values',version:1,setDigest,proposalIds:['prp_values'],reviewIds:reviews.map(x=>x.id),validationRunIds:[],replayed:false});
  const response=body=>({status:200,body:structuredClone(body)});
  const f={parent,root,state,original,originalRelease,before,data,connection,bindingOperation,bindingRelease,calls,view:()=>({state,original,originalRevision:structuredClone(baseline),originalRelease:structuredClone(originalRelease),release:structuredClone(release),current:structuredClone(current)}),cleanup:()=>rmSync(parent,{recursive:true,force:true})};
  const request=async(_root,actor,method,path,body,key)=>{
    const call={actor,method,path,body:structuredClone(body),key}; calls.push(call);
    const overridden=await f.override?.(call); if(overridden)return overridden;
    if(method==='POST'){
      assert.ok(lstatSync(join(root,'value-knowledge.json')).isFile());
      if(path.endsWith('/production-operations')){
        operation={summary:{id:'prodop_values',createdBy:'prn_author',progress:'draft',releaseId:null},version:1,setDigest,input:body.input,baselineHead:{presence:'present',releaseId:before.id,manifestDigest:before.afterManifest.digest},unresolvedCodes:[],activeValidation:{status:'not_started'},targets:[{declaration:body.targets[0],localKey:'model',targetId:'ast_model',contentDigest:hash('supplement'),outcome:'proposal',proposalId:'prp_values',proposalState:'draft',reviewIds:[]}]};
        return {status:201,body:command()};
      }
      if(path.endsWith('/business-rule-confirmations')){event={operationId:'prodop_values',productionVersion:1,setDigest,targetKey:'model',action:'confirm',principalId:'prn_author',contentDigest:hash('supplement'),replayed:false}; return {status:201,body:structuredClone(event)};}
      if(path.endsWith('/submit')){operation.activeValidation=validation;return {status:202,body:command()};}
      if(path.endsWith('/reviews')){reviews=[{id:'rvw_values',proposalId:'prp_values',decision:'approved',reviewerPrincipalId:'prn_reviewer'}];operation.targets[0].reviewIds=['rvw_values'];return response(command());}
      if(path.endsWith('/publish')){
        release={id:'rls_values',publishedBy:'prn_publisher',beforeHead:body.expectedHead,beforeManifest:before.afterManifest,beforePins:[{presence:'present',kind:'semantic_asset',targetId:'ast_model',revisionId:'rev_original',contentDigest:hash('original')}],afterManifest:{...structuredClone(before.afterManifest),digest:hash('after'),assets:before.afterManifest.assets.map(x=>x.assetId==='ast_model'?{...x,revisionId:'rev_values'}:x)},originProposalIds:['prp_values'],attribution:{operationId:'prodop_values',version:1,setDigest,validation:body.validation,proposalIds:['prp_values'],reviewIds:['rvw_values'],contributors:['prn_author'],role:'applied'}};
        current={assetId:'ast_model',id:'rev_values',content:supplementContent(original),contentDigest:hash('supplement')};operation.summary.progress='released';operation.summary.releaseId=release.id;operation.targets[0].proposalState='released';
        return {status:201,body:{operationId:'prodop_values',releaseId:release.id,manifestDigest:release.afterManifest.digest,replayed:false}};
      }
      throw new Error('Unexpected mutation');
    }
    if(path.endsWith('/governance/releases?limit=1'))return response({items:[{id:release.id}]});
    if(path.endsWith('/production-releases/rls_initial'))return response(originalRelease);
    if(path.endsWith('/production-releases/rls_binding'))return response(bindingRelease);
    if(path.endsWith('/production-releases/rls_current'))return response(before);
    if(path.endsWith('/production-releases/'+release.id))return response(release);
    if(path.endsWith('/revisions/rev_original'))return response(baseline);
    if(path.endsWith('/revisions/'+current.id))return response(current);
    if(path.endsWith('/revisions/rev_data'))return response(data);
    if(path.endsWith('/production-operations/prodop_original'))return response(originalOperation);
    if(path.endsWith('/production-operations/prodop_binding'))return response(bindingOperation);
    if(path.endsWith('/production-operations/prodop_values'))return response(operation);
    if(path.endsWith('/business-rule-confirmations?version=1'))return response({items:[{valid:true,event,declaration:valueGlossarySuffix.trim()}]});
    if(path.endsWith('/governance/proposals/prp_values/reviews'))return response({items:reviews});
    if(path.endsWith('/governance/proposals/prp_values'))return response({id:'prp_values',createdBy:'prn_author',state:'released',baseRevisionId:'rev_original',assetId:'ast_model'});
    if(path.endsWith('/sources/src_synthetic'))return response(connection);
    if(path.endsWith('/sources/src_synthetic/snapshots/snp_source'))return response(state.discovery.snapshot);
    throw new Error('Unexpected read path');
  };
  const rawSource={role:state.owner+'_reader',database:state.owner+'_source',readOnly:'on',schemaMarker:state.owner+':'+state.sourceDataDigest,values:['华东','华南']};
  f.rawSource=rawSource;
  f.options={request,counts:()=>({schema:33,unfinished:0,modelSteps:5,agentRuns:5,executions:4}),readCategories:async(_root,binding)=>validateSourceCategories(rawSource,state,binding),sleep:async()=>{},print:()=>{}};
  return f;
}

async function failedFixture() {
  const f=fixture(),receipt=await prepareValues(f.root,f.options);
  receipt.state='failed';receipt.complete=false;writePrivate(join(f.root,'value-knowledge.json'),receipt);rmSync(join(f.root,'value-knowledge-approved.json'));
  const command={startedAt:new Date(Date.parse(receipt.startedAt)-1).toISOString(),finishedAt:new Date(Date.parse(receipt.finishedAt)+1).toISOString(),exit:1,signal:null,stdout:'',stderr:'Protected preparation failure'};
  writePrivate(join(f.root,'value-knowledge-root-command.json'),command,true);
  for(const name of ['before','proof-failure','after-failed'])writePrivate(join(f.root,`value-knowledge-root-${name}.json`),{privateHistoricalEvidence:name},true);
  f.calls.length=0;f.failed=receipt;return f;
}

test('value knowledge is only the declared field-specific two-value glossary, not a question or answer', () => {
  const original = { assetType: 'analysis_model', definition: 'Synthetic governed model.', scope: 'Synthetic only.', spec: { memberBindings: [{ stable: true }] } };
  const result = supplementContent(original);
  assert.equal(result.definition, original.definition + valueGlossarySuffix);
  assert.deepEqual({ ...result, definition: original.definition }, original);
  assert.match(valueGlossarySuffix, /customers\.region/);
  assert.match(valueGlossarySuffix, /华东地区.*华东.*华南地区.*华南/);
  assert.doesNotMatch(valueGlossarySuffix, /200|600|客单价|2026年|8月|订单数|金额/);
  assert.throws(() => supplementContent(result));
});

test('offline ordinary governance creates exactly one definition-only supplement and preserves initial files and old ledgers', async()=>{
  const f=fixture();
  try{
    const names=['fixture.json','state.json','final-candidate.json','model-acceptance.json','correction-amount-acceptance.json'], before=names.map(n=>({n,bytes:readFileSync(join(f.root,n)),stat:lstatSync(join(f.root,n))}));
    const receipt=await prepareValues(f.root,f.options);
    assert.equal(receipt.complete,true); assert.deepEqual(f.calls.filter(x=>x.method==='POST').map(x=>x.actor),['author','author','author','reviewer','publisher']);
    assert.ok(f.calls.every(x=>!/(\/ask|:execute|semantic-queries|\/generation)/.test(x.path)));
    assert.deepEqual(receipt.steps[0].body.targets,[definitionOnlyTarget(f.original,f.before.afterManifest.assets[0])]);
    const proof=approvedKnowledgeBasis(f.root,f.view()); assert.equal(proof.kind,'approved-values'); assert.equal(proof.releaseId,'rls_values');
    for(const old of before){assert.deepEqual(readFileSync(join(f.root,old.n)),old.bytes);const stat=lstatSync(join(f.root,old.n));assert.equal(stat.ino,old.stat.ino);assert.equal(stat.mtimeMs,old.stat.mtimeMs);assert.equal(stat.ctimeMs,old.stat.ctimeMs);}
    const count=f.calls.length;await assert.rejects(()=>prepareValues(f.root,f.options));assert.equal(f.calls.length,count);
  }finally{f.cleanup();}
});

test('strict supplement proof rejects content, category source, manifest, independent review and partial publication drift',async()=>{
  const f=fixture();
  try{
    const good=await prepareValues(f.root,f.options); assert.equal(verifySupplementReceipt(good,f.view()).kind,'approved-values');
    const mutations=[r=>{r.current.content.spec.unchangedRule.left='changed';},r=>{r.suffix+='other';},r=>{r.original.definition='other';},r=>{r.source.before.values=['华东地区','华南'];},r=>{r.source.binding.field.objectId='fld_wrong';},r=>{r.source.data.content.spec.datasetRef.objectId='ds_wrong';},r=>{r.beforeRelease.afterManifest.objects[1].objectVersion++;},r=>{r.afterRelease.afterManifest.objects[1].objectVersion++;},r=>{r.reviews.items[0].reviewerPrincipalId='prn_author';},r=>{r.steps[3].response.reviewIds=[];},r=>{r.afterRelease.beforeHead.releaseId='rls_stale';},r=>{r.steps[4].body.expectedHead.manifestDigest=hash('stale');},r=>{r.complete=false;},r=>{r.operation.summary.releaseId=null;},r=>{r.confirmations.items[0].valid=false;},r=>{r.afterCounts.modelSteps++;}];
    for(const change of mutations){const value=structuredClone(good);change(value);assert.throws(()=>verifySupplementReceipt(value,f.view()));}
  }finally{f.cleanup();}
});

test('unknown mutation outcome is durable, never retried or resumed, and never creates an approved pointer',async()=>{
  for(const phase of ['create','confirm','submit','review','publish']){
    const f=fixture();
    try{
      const endings={create:'/production-operations',confirm:'/business-rule-confirmations',submit:'/submit',review:'/reviews',publish:'/publish'};
      f.override=call=>call.method==='POST'&&call.path.endsWith(endings[phase])?{status:0,body:{code:'HTTP_OUTCOME_UNKNOWN'}}:undefined;
      await assert.rejects(()=>prepareValues(f.root,f.options));const prior=readFileSync(join(f.root,'value-knowledge.json')), count=f.calls.length;
      assert.equal(JSON.parse(prior).state,'unknown');assert.equal(JSON.parse(prior).complete,false);assert.equal(lstatSync(join(f.root,'value-knowledge-approved.json'),{throwIfNoEntry:false}),undefined);
      await assert.rejects(()=>prepareValues(f.root,f.options));assert.equal(f.calls.length,count);assert.deepEqual(readFileSync(join(f.root,'value-knowledge.json')),prior);
      assert.throws(()=>approvedKnowledgeBasis(f.root,f.view()));
    }finally{f.cleanup();}
  }
});

test('source categories are read by the dedicated reader with password only in a private child environment',()=>{
  const f=fixture();
  try{
    const binding=sourceBindingProof(f.state,f.original,f.data,f.connection,f.state.discovery.snapshot,f.bindingOperation,f.before,f.bindingRelease);
    let invoked=0;
    const result=readSourceCategories(f.root,binding,{provision:()=>({container:'synthetic_container'}),environment:{PATH:'/synthetic',HOME:f.parent,UNRELATED_SECRET:'not-inherited'},spawn:(command,args,options)=>{
      invoked++;assert.equal(command,'docker');assert.equal(args.includes('f'.repeat(64)),false);assert.equal(options.env.PGPASSWORD,'f'.repeat(64));assert.equal(options.env.UNRELATED_SECRET,undefined);assert.equal(options.input,categorySQL);assert.match(options.input,/BEGIN READ ONLY/);assert.match(options.input,/SELECT DISTINCT region/);assert.equal(args[args.indexOf('-U')+1],f.state.owner+'_reader');return{status:0,stdout:JSON.stringify(f.rawSource)};
    }});
    assert.equal(invoked,1);assert.deepEqual(result.values,['华东','华南']);assert.equal(result.queryDigest,digest(categorySQL));
    assert.throws(()=>validateSourceCategories({...f.rawSource,role:f.state.owner},f.state,binding));assert.throws(()=>validateSourceCategories({...f.rawSource,values:['华东','华南','other']},f.state,binding));
  }finally{f.cleanup();}
});

test('original head proof remains strict and reads old correction evidence without modifying bytes or metadata',async()=>{
  const f=fixture();
  try{
    const path=join(f.root,'correction-amount-acceptance.json'),before=readFileSync(path),stat=lstatSync(path);
    const proof=await governanceHeadProof(f.root,{request:f.options.request});assert.equal(proof.correctBaselineContent,true);
    assert.deepEqual(readFileSync(path),before);assert.equal(lstatSync(path).ino,stat.ino);assert.equal(lstatSync(path).mtimeMs,stat.mtimeMs);assert.equal(lstatSync(path).ctimeMs,stat.ctimeMs);
    assert.equal(proof.knowledgeBasis.kind,'original');assert.equal(proof.originalBaselineContent,true);
    f.override=call=>call.path.endsWith('/revisions/rev_original')?{status:200,body:{...f.view().current,content:{...f.original,definition:'Unapproved arbitrary definition'}}}:undefined;
    await assert.rejects(()=>governanceHeadProof(f.root,{request:f.options.request}));
  }finally{f.cleanup();}
});

test('head proof accepts only an exact approved supplement and declares its non-original basis',async()=>{
  const f=fixture();
  try{
    await prepareValues(f.root,f.options);
    const proof=await governanceHeadProof(f.root,{request:f.options.request});assert.equal(proof.correctBaselineContent,true);assert.equal(proof.originalBaselineContent,false);assert.equal(proof.knowledgeBasis.kind,'approved-values');
    assert.equal(proof.knowledgeBasis.receiptDigest,digest(readFileSync(join(f.root,'value-knowledge.json'))));assert.equal(proof.knowledgeBasis.pointerDigest,digest(readFileSync(join(f.root,'value-knowledge-approved.json'))));
  }finally{f.cleanup();}
});

test('wx claim excludes a simultaneous preparation before any competing API or source read',async()=>{
  const f=fixture();
  try{
    let release;const barrier=new Promise(resolve=>{release=resolve;});let entered;const ready=new Promise(resolve=>{entered=resolve;});
    const first=prepareValues(f.root,{...f.options,readCategories:async(...args)=>{entered();await barrier;return f.options.readCategories(...args);}});
    await ready;const count=f.calls.length;await assert.rejects(()=>prepareValues(f.root,f.options));assert.equal(f.calls.length,count);
    release();assert.equal((await first).complete,true);assert.equal(f.calls.filter(x=>x.method==='POST').length,5);
  }finally{f.cleanup();}
});

test('partial, changed, non-private and symlinked supplement evidence is never adopted or overwritten',async()=>{
  for(const mutation of ['partial','pointer','receipt','permissions','symlink','dangling']){
    const f=fixture();
    try{
      await prepareValues(f.root,f.options);const ledger=join(f.root,'value-knowledge.json'),pointer=join(f.root,'value-knowledge-approved.json');
      if(mutation==='partial')rmSync(pointer);
      if(mutation==='pointer')writePrivate(pointer,{format:1,owner:f.state.owner,receipt:'value-knowledge.json',receiptDigest:hash('foreign'),passed:true});
      if(mutation==='receipt'){const r=JSON.parse(readFileSync(ledger));r.complete=false;writePrivate(ledger,r);}
      if(mutation==='permissions')chmodSync(pointer,0o644);
      if(['symlink','dangling'].includes(mutation)){renameSync(pointer,pointer+'.original');symlinkSync(mutation==='symlink'?pointer+'.original':pointer+'.missing',pointer);}
      const bytes=readFileSync(ledger),count=f.calls.length;assert.throws(()=>approvedKnowledgeBasis(f.root,f.view()));await assert.rejects(()=>prepareValues(f.root,f.options));assert.equal(f.calls.length,count);assert.deepEqual(readFileSync(ledger),bytes);
    }finally{f.cleanup();}
  }
});

test('root, owned ledger and prior evidence identity drift fail closed before any mutation',async()=>{
  for(const mutation of ['root','ledger','history']){
    const f=fixture();
    try{
      let destination;
      await assert.rejects(()=>prepareValues(f.root,{...f.options,readCategories:async(...args)=>{
        const result=await f.options.readCategories(...args);
        if(mutation==='root'){renameSync(f.root,f.root+'.original');mkdirSync(f.root,{mode:0o700});destination=join(f.root,'value-knowledge.json');writePrivate(destination,{foreign:true},true);}
        else {destination=join(f.root,mutation==='ledger'?'value-knowledge.json':'correction-amount-acceptance.json');renameSync(destination,destination+'.original');writePrivate(destination,{foreign:true},true);}
        return result;
      }}));
      assert.deepEqual(JSON.parse(readFileSync(destination)),{foreign:true});assert.equal(f.calls.filter(x=>x.method==='POST').length,0);
    }finally{f.cleanup();}
  }
});

test('new cohort inventory appearing during preparation cannot escape the preserved evidence boundary',async()=>{
  const f=fixture();
  try{
    await assert.rejects(()=>prepareValues(f.root,{...f.options,readCategories:async(...args)=>{
      const result=await f.options.readCategories(...args),directory=join(f.root,'final-candidates');
      if(!lstatSync(directory,{throwIfNoEntry:false})){mkdirSync(directory,{mode:0o700});writePrivate(join(directory,'foreign.json'),{foreign:true},true);}
      return result;
    }}));
    assert.equal(f.calls.filter(x=>x.method==='POST').length,0);assert.deepEqual(JSON.parse(readFileSync(join(f.root,'final-candidates','foreign.json'))),{foreign:true});
  }finally{f.cleanup();}
});

test('unverified source categories and stale current head refuse creation while retaining a one-shot failed claim',async()=>{
  for(const mutation of ['values','binding','head']){
    const f=fixture();
    try{
      if(mutation==='values')f.rawSource.values=['华东地区','华南'];
      if(mutation==='binding')f.data.content.spec.members[0].sourceFieldRef.objectId='fld_other';
      if(mutation==='head')f.override=call=>call.path.endsWith('/revisions/rev_original')?{status:200,body:{...f.view().current,content:{...f.original,definition:'Unapproved'}}}:undefined;
      await assert.rejects(()=>prepareValues(f.root,f.options));assert.equal(f.calls.filter(x=>x.method==='POST').length,0);
      assert.equal(JSON.parse(readFileSync(join(f.root,'value-knowledge.json'))).complete,false);const count=f.calls.length;await assert.rejects(()=>prepareValues(f.root,f.options));assert.equal(f.calls.length,count);
    }finally{f.cleanup();}
  }
});

test('preflight anchors every asset, object, model revision and digest to the fixed original release before any POST',async()=>{
  for(const mutation of ['asset','object','model','digest']){
    const f=fixture();
    try{
      const drift=structuredClone(f.before);
      if(mutation==='asset')drift.afterManifest.assets[1].revisionId='rev_other';
      if(mutation==='object')drift.afterManifest.objects[1].objectVersion++;
      if(mutation==='model')drift.afterManifest.assets[0].revisionId='rev_same_content';
      if(mutation==='digest')drift.afterManifest.digest=hash('different');
      f.override=call=>call.path.endsWith('/production-releases/rls_current')?{status:200,body:drift}:call.path.endsWith('/revisions/rev_same_content')?{status:200,body:{...f.view().current,id:'rev_same_content'}}:undefined;
      await assert.rejects(()=>prepareValues(f.root,f.options));assert.equal(f.calls.filter(x=>x.method==='POST').length,0,mutation);
      await assert.rejects(()=>governanceHeadProof(f.root,{request:f.options.request}));
    }finally{f.cleanup();}
  }
});

test('approved proof binds the complete fixed original release rather than a receipt-supplied replacement',async()=>{
  const f=fixture();
  try{
    const receipt=await prepareValues(f.root,f.options);
    for(const change of [r=>{r.originalRelease={...f.originalRelease,id:'rls_other'};},r=>{r.originalRelease=structuredClone(f.originalRelease);r.originalRelease.afterManifest.objects[1].objectVersion++;}]){
      const modified=structuredClone(receipt);change(modified);assert.throws(()=>verifySupplementReceipt(modified,f.view()));
    }
    const latest=structuredClone(receipt),view=f.view();
    latest.beforeRelease.afterManifest.assets[1].revisionId='rev_other';latest.afterRelease.beforeManifest.assets[1].revisionId='rev_other';latest.afterRelease.afterManifest.assets[1].revisionId='rev_other';view.release=latest.afterRelease;
    assert.throws(()=>verifySupplementReceipt(latest,view));
    f.override=call=>call.path.endsWith('/production-releases/rls_initial')?{status:200,body:{...f.originalRelease,sequence:999}}:undefined;
    await assert.rejects(()=>governanceHeadProof(f.root,{request:f.options.request}));
  }finally{f.cleanup();}
});

test('the fixed original API release must contain exactly every immutable state published pin',async()=>{
  for(const mutate of [r=>{r.afterManifest.assets[1].revisionId='rev_other';},r=>{r.afterManifest.objects[1].objectVersion++;},r=>{r.afterManifest.assets.push({assetId:'ast_extra',revisionId:'rev_extra'});},r=>{r.afterManifest.objects.pop();}]){
    const f=fixture();
    try{
      const altered=structuredClone(f.originalRelease);mutate(altered);
      f.override=call=>call.path.endsWith('/production-releases/rls_initial')?{status:200,body:altered}:undefined;
      await assert.rejects(()=>prepareValues(f.root,f.options));assert.equal(f.calls.filter(x=>x.method==='POST').length,0);
    }finally{f.cleanup();}
  }
});

test('binding content proof uses the released operation target when real manifest DTOs omit contentDigest',()=>{
  const f=fixture();
  try{
    assert.equal(f.before.afterManifest.objects[0].contentDigest,undefined);assert.equal(f.state.published.customers_binding.contentDigest,undefined);
    const proof=sourceBindingProof(f.state,f.original,f.data,f.connection,f.state.discovery.snapshot,f.bindingOperation,f.before,f.bindingRelease);
    assert.equal(proof.bindingContentDigest,hash('binding'));assert.equal(proof.bindingOperationId,'prodop_binding');assert.equal(proof.bindingOperationVersion,1);assert.equal(proof.bindingSetDigest,hash('binding-set'));
    for(const mutate of [op=>{op.targets[0].declaration.content.dataset.objectId='ds_wrong';},op=>{op.summary.id='prodop_wrong';},op=>{op.summary.releaseId='rls_wrong';},op=>{op.summary.progress='draft';},op=>{op.targets[0].contentDigest='';},op=>{op.targets[0].proposalState='approved';}]){
      const altered=structuredClone(f.bindingOperation);mutate(altered);assert.throws(()=>sourceBindingProof(f.state,f.original,f.data,f.connection,f.state.discovery.snapshot,altered,f.before,f.bindingRelease));
    }
    const changed=structuredClone(f.before);changed.afterManifest.objects[0].objectVersion++;assert.throws(()=>sourceBindingProof(f.state,f.original,f.data,f.connection,f.state.discovery.snapshot,f.bindingOperation,changed,f.bindingRelease));
    for(const mutate of [r=>{r.attribution.version++;},r=>{r.attribution.setDigest=hash('wrong');},r=>{r.attribution.operationId='prodop_wrong';},r=>{r.afterManifest.objects[0].objectVersion++;}]){
      const altered=structuredClone(f.bindingRelease);mutate(altered);assert.throws(()=>sourceBindingProof(f.state,f.original,f.data,f.connection,f.state.discovery.snapshot,f.bindingOperation,f.before,altered));
    }
  }finally{f.cleanup();}
});

test('release proof ignores only live projection status while retaining every immutable release fact',async()=>{
  const f=fixture();
  try{
    const receipt=await prepareValues(f.root,f.options),stored=structuredClone(receipt),view=f.view();
    stored.originalRelease.projectionStatus='pending';stored.afterRelease.projectionStatus='pending';
    view.originalRelease.projectionStatus='ready';view.release.projectionStatus='ready';
    assert.equal(verifySupplementReceipt(stored,view).kind,'approved-values');
    for(const mutate of [r=>{r.afterManifest.digest=hash('drift');},r=>{r.attribution.role='rollback';},r=>{r.beforePins[0].revisionId='rev_other';},r=>{r.publishedBy='prn_other';},r=>{r.id='rls_other';}]){
      const altered=structuredClone(view);mutate(altered.release);assert.throws(()=>verifySupplementReceipt(stored,altered));
    }
  }finally{f.cleanup();}
});

test('semantic before-pin digest is required and anchored to the fixed original revision and production target',async()=>{
  const f=fixture();
  try{
    const receipt=await prepareValues(f.root,f.options),view=f.view();
    assert.equal(receipt.afterRelease.beforePins[0].contentDigest,view.originalRevision.contentDigest);
    for(const mutate of [r=>{delete r.afterRelease.beforePins[0].contentDigest;},r=>{r.afterRelease.beforePins[0].contentDigest=hash('wrong');},r=>{r.originalOperation.targets[0].contentDigest=hash('wrong');},r=>{r.originalOperation.targets[0].targetId='ast_wrong';},r=>{r.originalOperation.targets[0].declaration.content.definition='changed';},r=>{r.originalOperation.summary.releaseId='rls_wrong';}]){
      const altered=structuredClone(receipt);mutate(altered);assert.throws(()=>verifySupplementReceipt(altered,{...view,release:structuredClone(altered.afterRelease)}));
    }
    const changed=structuredClone(view);changed.originalRevision.contentDigest=hash('wrong');assert.throws(()=>verifySupplementReceipt(receipt,changed));
  }finally{f.cleanup();}
});

test('read-only reconciliation verifies a known applied publication without rewriting the failed command or issuing POST',async()=>{
  const f=await failedFixture();
  try{
    const names=['value-knowledge.json','value-knowledge-root-command.json','value-knowledge-root-before.json','value-knowledge-root-proof-failure.json','value-knowledge-root-after-failed.json','fixture.json','state.json','final-candidate.json','correction-amount-acceptance.json'];
    const before=names.map(name=>({name,bytes:readFileSync(join(f.root,name)),stat:lstatSync(join(f.root,name))}));
    assert.equal(verifySupplementFacts(f.failed,f.view()).kind,'approved-values');assert.throws(()=>verifySupplementReceipt(f.failed,f.view()));
    const result=await reconcileValues(f.root,f.options);assert.equal(result.complete,true);assert.equal(result.kind,'read-only-reconciliation');
    assert.equal(result.originalWrapper.exit,1);assert.equal(result.originalWrapper.receiptDigest,digest(before[0].bytes));
    assert.ok(f.calls.length>0);assert.ok(f.calls.every(x=>x.method==='GET'&&!/(\/ask|:execute|\/test|\/generation)/.test(x.path)));
    const basis=approvedKnowledgeBasis(f.root,f.view());assert.equal(basis.verification,'read-only-reconciliation');assert.equal(basis.failedReceiptDigest,digest(before[0].bytes));assert.equal(basis.rootCommandDigest,digest(before[1].bytes));
    for(const old of before){const path=join(f.root,old.name),stat=lstatSync(path);assert.deepEqual(readFileSync(path),old.bytes);for(const key of ['ino','mtimeMs','ctimeMs'])assert.equal(stat[key],old.stat[key]);}
    const count=f.calls.length;await assert.rejects(()=>reconcileValues(f.root,f.options));assert.equal(f.calls.length,count);
    assert.equal(JSON.parse(readFileSync(join(f.root,'value-knowledge.json'))).complete,false);
  }finally{f.cleanup();}
});

test('reconciliation rejects unknown, partial or differently published preparation without workspace requests',async()=>{
  for(const mutate of [r=>{r.steps[4].state='unknown';},r=>{r.steps.pop();},r=>{r.steps[3].status=500;},r=>{r.steps[4].response.replayed=true;},r=>{r.afterRelease.id='rls_other';}]){
    const f=await failedFixture();
    try{
      const changed=structuredClone(f.failed);mutate(changed);writePrivate(join(f.root,'value-knowledge.json'),changed);const bytes=readFileSync(join(f.root,'value-knowledge.json'));
      await assert.rejects(()=>reconcileValues(f.root,f.options));assert.equal(f.calls.length,0);assert.deepEqual(readFileSync(join(f.root,'value-knowledge.json')),bytes);assert.equal(lstatSync(join(f.root,'value-knowledge-approved.json'),{throwIfNoEntry:false}),undefined);
    }finally{f.cleanup();}
  }
});

test('fresh readback drift never approves or retries a reconciliation and preserves the applied failed attempt',async()=>{
  for(const mutation of ['head','content','reviewer','manifest','source','counts']){
    const f=await failedFixture();
    try{
      const before=readFileSync(join(f.root,'value-knowledge.json'));
      if(mutation==='head')f.override=call=>call.path.endsWith('/governance/releases?limit=1')?{status:200,body:{items:[{id:'rls_other'}]}}:undefined;
      if(mutation==='content')f.override=call=>call.path.endsWith('/revisions/rev_values')?{status:200,body:{...f.view().current,content:{...f.original,definition:'drift'}}}:undefined;
      if(mutation==='reviewer')f.override=call=>call.path.endsWith('/governance/proposals/prp_values/reviews')?{status:200,body:{items:[{id:'rvw_values',proposalId:'prp_values',decision:'approved',reviewerPrincipalId:'prn_author'}]}}:undefined;
      if(mutation==='manifest')f.override=call=>call.path.endsWith('/production-releases/rls_values')?{status:200,body:{...f.view().release,afterManifest:{...f.view().release.afterManifest,digest:hash('wrong')}}}:undefined;
      if(mutation==='source')f.rawSource.values=['unexpected'];
      let countCalls=0;const options={...f.options,...(mutation==='counts'?{counts:()=>({schema:33,unfinished:0,modelSteps:5+(countCalls++>0?1:0),agentRuns:5,executions:4})}:{})};
      await assert.rejects(()=>reconcileValues(f.root,options));assert.ok(f.calls.every(x=>x.method==='GET'));assert.deepEqual(readFileSync(join(f.root,'value-knowledge.json')),before);assert.equal(lstatSync(join(f.root,'value-knowledge-approved.json'),{throwIfNoEntry:false}),undefined);
      const calls=f.calls.length;await assert.rejects(()=>reconcileValues(f.root,options));assert.equal(f.calls.length,calls);
    }finally{f.cleanup();}
  }
});

test('reconciled approval binds both receipts and the original failed root command with private path guards',async()=>{
  for(const name of ['value-knowledge.json','value-knowledge-reconciliation.json','value-knowledge-root-command.json','value-knowledge-approved.json']){
    const f=await failedFixture();
    try{
      await reconcileValues(f.root,f.options);const path=join(f.root,name);renameSync(path,path+'.original');symlinkSync(path+'.original',path);
      assert.throws(()=>approvedKnowledgeBasis(f.root,f.view()));
    }finally{f.cleanup();}
  }
});

test('a verified reconciliation preserves historical cohorts but permits a later separately guarded candidate ledger',async()=>{
  const f=await failedFixture();
  try{
    await reconcileValues(f.root,f.options);const prior=approvedKnowledgeBasis(f.root,f.view());
    const directory=join(f.root,'final-candidates');mkdirSync(directory,{mode:0o700});writePrivate(join(directory,'.owner.json'),{format:1,owner:f.state.owner},true);writePrivate(join(directory,'new-candidate.json'),{new:true},true);
    assert.deepEqual(approvedKnowledgeBasis(f.root,f.view()),prior);
  }finally{f.cleanup();}
});

test('reconciliation claim and preserved identities fence concurrent, partial, loose and replaced evidence',async()=>{
  for(const mutation of ['existing','permissions','root-command','claim','inventory']){
    const f=await failedFixture();
    try{
      const path=join(f.root,'value-knowledge-reconciliation.json');
      if(mutation==='existing')writePrivate(path,{state:'claimed'},true);
      if(mutation==='permissions')chmodSync(join(f.root,'value-knowledge-root-command.json'),0o644);
      let once=false;
      f.override=()=>{
        if(once)return;once=true;
        if(mutation==='root-command')writePrivate(join(f.root,'value-knowledge-root-command.json'),{foreign:true});
        if(mutation==='claim'){renameSync(path,path+'.original');writePrivate(path,{foreign:true},true);}
        if(mutation==='inventory'){const directory=join(f.root,'final-candidates');mkdirSync(directory,{mode:0o700});writePrivate(join(directory,'unexpected.json'),{foreign:true},true);}
      };
      const before=readFileSync(join(f.root,'value-knowledge.json'));await assert.rejects(()=>reconcileValues(f.root,f.options));assert.deepEqual(readFileSync(join(f.root,'value-knowledge.json')),before);
      assert.equal(lstatSync(join(f.root,'value-knowledge-approved.json'),{throwIfNoEntry:false}),undefined);assert.ok(f.calls.every(x=>x.method==='GET'));
      if(['existing','permissions'].includes(mutation))assert.equal(f.calls.length,0);
      if(mutation==='claim')assert.deepEqual(JSON.parse(readFileSync(path)),{foreign:true});
    }finally{f.cleanup();}
  }
});

test('a second concurrent readback cannot pass the wx claim or touch the saved failed attempt',async()=>{
  const f=await failedFixture();
  try{
    let release,entered;const barrier=new Promise(resolve=>{release=resolve;}),ready=new Promise(resolve=>{entered=resolve;});
    let once=false;f.override=async()=>{if(!once){once=true;entered();await barrier;}};
    const first=reconcileValues(f.root,f.options);await ready;const calls=f.calls.length;
    await assert.rejects(()=>reconcileValues(f.root,f.options));assert.equal(f.calls.length,calls);release();assert.equal((await first).complete,true);
    assert.equal(JSON.parse(readFileSync(join(f.root,'value-knowledge.json'))).state,'failed');
  }finally{f.cleanup();}
});

test('receipt, pointer and failed command bytes remain bound after read-only approval',async()=>{
  for(const name of ['value-knowledge.json','value-knowledge-reconciliation.json','value-knowledge-root-command.json','value-knowledge-approved.json']){
    const f=await failedFixture();
    try{
      await reconcileValues(f.root,f.options);const path=join(f.root,name),value=JSON.parse(readFileSync(path));value.changed=true;writePrivate(path,value);
      assert.throws(()=>approvedKnowledgeBasis(f.root,f.view()));
    }finally{f.cleanup();}
  }
});
