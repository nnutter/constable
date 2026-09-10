package report

import "fmt"

func MutatesPointerParameter(name string) string {
	return fmt.Sprintf("//constable:nonmutating function mutates pointer parameter %s", name)
}

func MutatesReceiver(name string) string {
	return fmt.Sprintf("//constable:nonmutating method mutates receiver %s", name)
}

func MutatesParameter(name string) string {
	return fmt.Sprintf("//constable:nonmutating function mutates parameter %s", name)
}

func DeletesFromMapParameter(name string) string {
	return fmt.Sprintf("//constable:nonmutating function deletes from map parameter %s", name)
}

func MethodShouldBeInSameFile(typeName, methodName string) string {
	return fmt.Sprintf("method %s of type %s should be in same file as type definition", methodName, typeName)
}

func MethodShouldBeSorted(typeName, methodName, otherMethod string) string {
	return fmt.Sprintf("method %s of type %s should be sorted before method %s", methodName, typeName, otherMethod)
}

func UseTestifyInstead(testingMethod, testifyPackage string) string {
	return fmt.Sprintf("use testify/%s instead of testing.%s", testifyPackage, testingMethod)
}

func ComplexityExceedsLimit(name string, complexity, limit int) string {
	return fmt.Sprintf("cyclomatic complexity %d exceeds limit %d (function %s)", complexity, limit, name)
}
