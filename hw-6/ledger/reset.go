package ledger

func Reset() {
	mu.Lock()
	defer mu.Unlock()

	transactions = []Transaction{}
	budgets = map[string]Budget{}
}
