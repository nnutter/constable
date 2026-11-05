// want package:`sumTypeFact\{Shape \[Circle Square\]\}`
package st

//sumtype:decl
type Shape interface{ shape() }

type Circle struct{}

func (*Circle) shape() {}

type Square struct{}

func (*Square) shape() {}

func F(s Shape) {
	switch s.(type) { // want `missing cases for Square`
	case *Circle:
	}
}
