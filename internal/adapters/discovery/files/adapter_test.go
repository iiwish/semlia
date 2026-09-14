package files_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	filesadapter "github.com/iiwish/semlia/internal/adapters/discovery/files"
	"github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/internal/domain/ingestion"
)

func TestCSVPreservesMaximumUploadNameInCoverage(t *testing.T) {
	for _, length := range []int{251, 252, 255} {
		name := strings.Repeat("x", length-4) + ".csv"
		snapshot, err := filesadapter.CSV().Discover(context.Background(), input(name, []byte("id\n1\n")))
		if err != nil {
			t.Fatalf("valid %d-byte upload name rejected: %v", length, err)
		}
		if len(snapshot.Coverage) != 1 || len(snapshot.Coverage[0].Key) > 256 || snapshot.Coverage[0].Selector != name || snapshot.Datasets[0].CoverageKey != snapshot.Coverage[0].Key {
			t.Fatalf("coverage=%+v", snapshot.Coverage)
		}
	}
}

func TestCSVRejectsInconsistentRowsAndActiveCells(t *testing.T) {
	t.Parallel()
	for _, content := range []string{"a,b\n1\n", "a,b\n=cmd,2\n", "a,b\n1,\x00\n", "account\u202Etxt,value\n1,2\n"} {
		_, err := filesadapter.CSV().Discover(context.Background(), input("input.csv", []byte(content)))
		if !errors.Is(err, ingestion.ErrUnsafeContent) {
			t.Fatalf("content %q: %v", content, err)
		}
	}
}

func TestXLSXValidatesPackageAndRejectsExecutableFeatures(t *testing.T) {
	t.Parallel()
	valid := workbook(map[string]string{
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>name</t></is></c></row><row r="2"><c r="A2"><v>value</v></c></row></sheetData></worksheet>`,
	})
	if snapshot, err := filesadapter.XLSX().Discover(context.Background(), input("input.xlsx", valid)); err != nil || len(snapshot.Datasets) != 1 {
		t.Fatalf("valid workbook: datasets=%d err=%v", len(snapshot.Datasets), err)
	}
	for name, value := range map[string][]byte{
		"formula":  workbook(map[string]string{"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1"><f>NOW()</f><v>1</v></c></row></sheetData></worksheet>`}),
		"sparse":   workbook(map[string]string{"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="XFD1"><v>x</v></c></row></sheetData></worksheet>`}),
		"external": workbook(map[string]string{"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1"><v>x</v></c></row></sheetData></worksheet>`, "xl/worksheets/_rels/sheet1.xml.rels": `<Relationships><Relationship TargetMode='External' Target='https://example.test'/></Relationships>`}),
		"executable-part": workbook(map[string]string{
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1"><v>x</v></c></row></sheetData></worksheet>`,
			"payload.exe":              "MZ executable",
		}),
		"script-part": workbook(map[string]string{
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1"><v>x</v></c></row></sheetData></worksheet>`,
			"xl/payload.js":            "alert(1)",
		}),
		"html-part": workbook(map[string]string{
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1"><v>x</v></c></row></sheetData></worksheet>`,
			"docProps/payload.html":    "<script>alert(1)</script>",
		}),
		"dangerous-content-type": workbook(map[string]string{
			"[Content_Types].xml":      `<Types><Default Extension="js" ContentType="text/javascript"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/></Types>`,
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1"><v>x</v></c></row></sheetData></worksheet>`,
		}),
		"bidi": workbook(map[string]string{"xl/workbook.xml": `<workbook xmlns:r="urn:rels"><sheets><sheet name="Sheet` + "\u202e" + `txt" r:id="rId1"/></sheets></workbook>`,
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1"><v>x</v></c></row></sheetData></worksheet>`}),
	} {
		if _, err := filesadapter.XLSX().Discover(context.Background(), input("input.xlsx", value)); !errors.Is(err, ingestion.ErrUnsafeContent) && !errors.Is(err, ingestion.ErrLimitExceeded) {
			t.Fatalf("%s workbook: %v", name, err)
		}
	}
}

func TestXLSXPreservesWorksheetIdentityInQualifiedNames(t *testing.T) {
	t.Parallel()
	value := workbook(map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="urn:rels"><sheets><sheet name="Orders" r:id="rId1"/><sheet name="Customers" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>order_id</t></is></c></row></sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml":   `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>customer_id</t></is></c></row></sheetData></worksheet>`,
	})
	snapshot, err := filesadapter.XLSX().Discover(context.Background(), input("warehouse.xlsx", value))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, dataset := range snapshot.Datasets {
		names[dataset.QualifiedName] = true
	}
	if len(snapshot.Datasets) != 2 || !names["warehouse.Orders"] || !names["warehouse.Customers"] {
		t.Fatalf("datasets=%+v", snapshot.Datasets)
	}
}

func TestXLSXRejectsFormulasOutsideReferencedCells(t *testing.T) {
	baseSheet := `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>id</t></is></c></row></sheetData></worksheet>`
	for _, name := range []string{"calculatedColumnFormula", "totalsRowFormula", "unreferenced_sheet"} {
		t.Run(name, func(t *testing.T) {
			parts := map[string]string{"xl/worksheets/sheet1.xml": baseSheet}
			if name == "unreferenced_sheet" {
				parts["xl/worksheets/sheet2.xml"] = `<worksheet><sheetData><row><c><f>NOW()</f><v>1</v></c></row></sheetData></worksheet>`
			} else {
				parts["xl/tables/table1.xml"] = `<table><tableColumns><tableColumn id="1" name="id"><` + name + `>NOW()</` + name + `></tableColumn></tableColumns></table>`
			}
			if _, err := filesadapter.XLSX().Discover(context.Background(), input("input.xlsx", workbook(parts))); !errors.Is(err, ingestion.ErrUnsafeContent) {
				t.Fatalf("package formula accepted: %v", err)
			}
		})
	}
}

func TestMarkdownRejectsExecutableContentAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	for _, content := range []string{"<script>alert(1)</script>", "[open](javascript:alert(1))", "![](data:text/html,bad)",
		`[open](javascript\:alert(1))`, `[open](jav&#x61;script:alert(1))`} {
		if _, err := filesadapter.Markdown().Discover(context.Background(), input("input.md", []byte(content))); !errors.Is(err, ingestion.ErrUnsafeContent) {
			t.Fatalf("content %q: %v", content, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := filesadapter.CSV().Discover(ctx, input("input.csv", []byte("a\n1\n"))); err == nil {
		t.Fatal("cancelled parse succeeded")
	}
}

func TestMarkdownRejectsEntityAndEscapeNodeFloodBeforeParsing(t *testing.T) {
	t.Parallel()
	for name, content := range map[string][]byte{
		"entities": bytes.Repeat([]byte("&amp;"), ingestion.MaxMarkdownNodes+1),
		"escapes":  bytes.Repeat([]byte("\\*"), ingestion.MaxMarkdownNodes+1),
	} {
		if _, err := filesadapter.Markdown().Discover(context.Background(), input("input.md", content)); !errors.Is(err, ingestion.ErrLimitExceeded) {
			t.Fatalf("%s flood: %v", name, err)
		}
	}
}

func input(name string, content []byte) discovery.Input {
	return discovery.Input{Locator: "artifact:test", ObservedAt: time.Now().UTC(), Files: map[string][]byte{name: content}}
}

func workbook(extra map[string]string) []byte {
	entries := map[string]string{
		"[Content_Types].xml":        `<Types><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/></Types>`,
		"xl/workbook.xml":            `<workbook xmlns:r="urn:rels"><sheets><sheet name="Sheet1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
	}
	for name, value := range extra {
		entries[name] = value
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, value := range entries {
		file, _ := writer.Create(name)
		_, _ = file.Write([]byte(value))
	}
	_ = writer.Close()
	return buffer.Bytes()
}
