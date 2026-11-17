package client

// This file has comments but no suppression directive.
func Safe(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
