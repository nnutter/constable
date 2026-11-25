package fix

var single = 1 // want "variable single is never modified and should be a constant"

var text string = "hello" // want "variable text is never modified and should be a constant"

func Use() int {
	var local = 2 // want "variable local is never modified and should be a constant"
	return single + local + len(text)
}
