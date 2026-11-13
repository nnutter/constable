package enum

//sumtype:decl
type CheckType interface {
	isCheckType()
}

func ParseCheckType(s string) CheckType {
	switch s {
	case NonmutatingName:
		return Nonmutating{}
	case PureName:
		return Pure{}
	default:
		return nil
	}
}

const NonmutatingName = "nonmutating"

type Nonmutating struct{}

func (Nonmutating) isCheckType() {}

const PureName = "pure"

type Pure struct{}

func (Pure) isCheckType() {}
