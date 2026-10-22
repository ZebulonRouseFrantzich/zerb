package profile

const (
	maxTemplateBytes   = 1 << 20
	maxTemplateActions = uint64(4096)
)

// Go 1.24's template parser keeps storage per call, without an input pool.
// Reserve source/text/error workspace, AST/list growth and fixed parser state.
func reserveTemplate(source []byte, actions uint64) bool {
	if len(source) > maxTemplateBytes || actions > maxTemplateActions {
		return false
	}
	var budget reservation
	return budget.add(1, 65_536) && budget.add(uint64(len(source)), 3) && budget.add(actions, 1024)
}
