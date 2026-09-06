package m

func (s Split) B() {} // want "method B of type Split should be in same file as type definition"

func (m *Mixed) C() {} // want "method C of type Mixed should be in same file as type definition"
