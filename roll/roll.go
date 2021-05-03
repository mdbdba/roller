package roll

import (
	"context"
	"fmt"
	"github.com/mdbdba/dice"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func IsTest(rollDesc string) (bool, int) {
	p1 := "5|7|9|11d1$"
	res, _ := regexp.MatchString(p1, rollDesc)
	rolls := 0
	if res {
		rolls, _ = strconv.Atoi(strings.Split(rollDesc, "d")[0])
	}
	return res, rolls
}

func GetRoll(ctx context.Context, logger *zap.Logger, roll string) dice.RollResult {
	span := oteltrace.SpanFromContext(ctx)
	// _, span := tracer.Start(ctx, "performRoll")
	span.AddEvent("callDiceRoll", oteltrace.WithAttributes(
		attribute.String("roll", roll)))

	res, _, _ := dice.Roll(roll)

	span.SetAttributes(attribute.Int("rollResult", res.Int()))
	span.End()

	span2 := oteltrace.SpanFromContext(ctx)
	// _, span2 := tracer.Start(ctx, "setAttributes")
	defer span2.End()
	time.Sleep(100 * time.Millisecond)
	auditStr := fmt.Sprintf("%s %s", roll, res.String())
	span2.SetAttributes(attribute.String("rollRequest", roll),
		attribute.Int("rollResult", res.Int()),
		attribute.String("rollAudit", auditStr))
	logger.Info("getRoll performed", zap.String("rollAudit", auditStr))

	return res
}
