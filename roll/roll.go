package roll

import (
	"context"
	"github.com/mdbdba/dice"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"time"
)

func getRoll(ctx context.Context, tracer oteltrace.Tracer, logger zap.Logger, roll string) dice.RollResult {
	_, span := tracer.Start(ctx, "performRoll")
	span.AddEvent("callDiceRoll", oteltrace.WithAttributes(
		attribute.String("roll", roll)))

	res, _, _ := dice.Roll(roll)

	span.SetAttributes(attribute.Int("rollResult", res.Int()))
	span.End()

	_, span2 := tracer.Start(ctx, "setAttributes")
	defer span2.End()
	time.Sleep(100 * time.Millisecond)
	auditStr := fmt.Sprintf("%s %s", roll, res.String())
	span2.SetAttributes(attribute.String("rollRequest", roll),
		attribute.Int("rollResult", res.Int()),
		attribute.String("rollAudit", auditStr))
	logger.Info("getRoll performed", zap.String("rollAudit", auditStr))

	return res
}
