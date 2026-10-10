package embedding

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestValidateVectorsRejectsIncompleteMixedAndNonFiniteBatches(t *testing.T) {
	for _, vectors := range [][][]float32{nil, {{1, 2}}, {{1, 2}, {3}}, {{1, 2}, {float32(math.NaN()), 3}}, {{0, 0}, {1, 2}}} {
		if ValidateVectors(vectors, 2, 2) == nil {
			t.Fatalf("accepted invalid batch %v", vectors)
		}
	}
	if err := ValidateVectors([][]float32{{1, 2}, {3, 4}}, 2, 2); err != nil {
		t.Fatal(err)
	}
}

func TestChunkTextIndexesGovernedWording(t *testing.T) {
	text, err := ChunkText("commerce.paid_amount", json.RawMessage(`{"displayName":"有效支付金额","definition":"已支付且未退款","aliases":["GMV","成交额"],"excludes":["已退款订单"],"sampleQuestions":[{"question":"8月 GMV 是多少？"}],"spec":{"members":[{"id":"region","name":"区域","aliases":["大区"]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "commerce.paid_amount\n有效支付金额\n已支付且未退款\naliases: GMV; 成交额\nexcludes: 已退款订单\nquestions: 8月 GMV 是多少？\nmembers: 区域/大区"
	if text != want {
		t.Fatalf("chunk text = %q", text)
	}
	long, err := ChunkText("a.b", json.RawMessage(`{"definition":"`+strings.Repeat("长", 1000)+`"}`))
	if err != nil || len(long) > 2048 || !utf8.ValidString(long) {
		t.Fatalf("chunk not bounded: %d %v", len(long), err)
	}
	if _, err := ChunkText("a.b", json.RawMessage(`[]`)); err == nil {
		t.Fatal("non-object content accepted")
	}
}
