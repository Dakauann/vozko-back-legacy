package conversation_usecase

import "testing"

func TestTheAutoCloseJobRefusesToBeBuiltWithoutItsStores(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("an auto close job without its stores was built")
		}
	}()
	NewAutoCloseJob(nil, nil)
}
