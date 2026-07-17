package state

var value = 40

func Add(amount int) {
	value += amount
}

func Value() int {
	return value
}
