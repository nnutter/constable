package lib

func Missing() *int {
	return nil //nolint:nilaway
}

func Dereference(p *int) int {
	return *p //nolint:nilaway
}

func Inline() int {
	var p *int
	return *p //nolint:nilaway
}

func Block() int {
	var p *int
	return *p /*nolint:nilaway*/
}

//nolint:all
func Declaration() int {
	var p *int
	return *p
}
