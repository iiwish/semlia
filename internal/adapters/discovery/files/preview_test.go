package files

import (
	"context"
	"strings"
	"testing"
)

func TestPreviewCSVPreservesRowsAndBounds(t *testing.T) {
	p, err := CSV().Preview(context.Background(), "people.csv", []byte("id,name\n1,甲\n2,\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Sheets) != 1 || len(p.Sheets[0].Rows) != 2 || p.Sheets[0].Rows[1].Cells[1] != "" || p.Sheets[0].Rows[0].Number != 2 {
		t.Fatalf("preview=%+v", p)
	}
	p, err = CSV().Preview(context.Background(), "many.csv", []byte("id,name\n"+strings.Repeat("1,hello\n", 120)))
	if err != nil || !p.Truncated || len(p.Sheets[0].Rows) != 100 {
		t.Fatalf("preview=%+v err=%v", p, err)
	}
	p, err = CSV().Preview(context.Background(), "exact.csv", []byte("id,name\n"+strings.Repeat("1,hello\n", 100)))
	if err != nil || p.Truncated || p.Datasets[0].Fields[1].DataType != "csv_text" {
		t.Fatalf("exact preview=%+v err=%v", p, err)
	}
}

func TestPreviewMarkdownSafeAndLocated(t *testing.T) {
	p, err := Markdown().Preview(context.Background(), "rules.md", []byte("# 指标\n\n收入为含税金额。\n"))
	if err != nil || p.Text != "# 指标\n\n收入为含税金额。\n" || len(p.Lines) != 3 || p.Lines[2].Number != 3 {
		t.Fatalf("preview=%+v err=%v", p, err)
	}
	if _, err := Markdown().Preview(context.Background(), "bad.md", []byte("<script>alert(1)</script>")); err == nil {
		t.Fatal("unsafe HTML accepted")
	}
	if len(p.Blocks) != 2 || p.Blocks[0].Kind != "heading" || p.Blocks[1].Line != 3 || p.Datasets[0].Fields[0].DataType != "markdown_text" {
		t.Fatalf("blocks=%+v datasets=%+v", p.Blocks, p.Datasets)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Markdown().Preview(ctx, "rules.md", []byte("# Rules")); err == nil {
		t.Fatal("canceled preview accepted")
	}
}
