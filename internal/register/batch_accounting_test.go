package register

import (
	"context"
	"testing"
)

func TestShouldRefundFailure(t *testing.T) {
	t.Run("non gmail refunds before max attempts", func(t *testing.T) {
		if !shouldRefundFailure(context.Background(), 2, 3, false, 0) {
			t.Fatal("expected refund before max attempts")
		}
	})

	t.Run("non gmail does not refund at max attempts", func(t *testing.T) {
		if shouldRefundFailure(context.Background(), 3, 3, false, 0) {
			t.Fatal("did not expect refund at max attempts")
		}
	})

	t.Run("cancelled context does not refund", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if shouldRefundFailure(ctx, 1, 3, false, 0) {
			t.Fatal("did not expect refund after cancellation")
		}
	})

	t.Run("gmail refunds only with remaining pool", func(t *testing.T) {
		if !shouldRefundFailure(context.Background(), 1, 3, true, 1) {
			t.Fatal("expected refund when gmail pool remains")
		}

		if shouldRefundFailure(context.Background(), 1, 3, true, 0) {
			t.Fatal("did not expect refund when gmail pool is empty")
		}
	})
}
