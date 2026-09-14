import { parseTypeId } from "./ids";
import type { AttentionItemId, SourceSnapshotId, SourceCodeRevisionId, SourceLineageRevisionId, ProductionOperationId } from "./ids";

const attentionItem: AttentionItemId = parseTypeId(
  "ati",
  "ati_01arz3ndektsv4rrffq69g5fav",
);

void attentionItem;
const sourceSnapshot: SourceSnapshotId = parseTypeId("ssnp", "ssnp_01arz3ndektsv4rrffq69g5fav");
const sourceCode: SourceCodeRevisionId = parseTypeId("codrev", "codrev_01arz3ndektsv4rrffq69g5fav");
const sourceLineage: SourceLineageRevisionId = parseTypeId("linrev", "linrev_01arz3ndektsv4rrffq69g5fav");
const productionOperation: ProductionOperationId = parseTypeId("prodop", "prodop_01arz3ndektsv4rrffq69g5fav");
void sourceSnapshot;
void sourceCode;
void sourceLineage;
void productionOperation;
