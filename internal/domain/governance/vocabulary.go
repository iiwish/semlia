package governance

import (
	"fmt"
	"strings"

	"github.com/iiwish/semlia/internal/domain/semantic"
)

// VocabularyConflictError reports terms that would name two released assets of one type.
type VocabularyConflictError struct {
	Conflicts []semantic.VocabularyConflict
}

func (e *VocabularyConflictError) Error() string {
	return fmt.Sprintf("vocabulary conflict on %d term(s)", len(e.Conflicts))
}

func (e *VocabularyConflictError) Unwrap() error { return ErrDependencyInvalid }

// Message names only the contested terms, never the other asset, which the caller may not be allowed to read.
func (e *VocabularyConflictError) Message() string {
	terms := []string{}
	seen := map[string]bool{}
	for _, conflict := range e.Conflicts {
		if !seen[conflict.Term] && len(terms) < 8 {
			seen[conflict.Term] = true
			terms = append(terms, "「"+conflict.Term+"」")
		}
	}
	return "别名冲突：" + strings.Join(terms, "、") + "会在发布后指向两个同类资产。请删去其中一个别名，或在任一方的易混淆项中声明对方并写明区别。"
}
