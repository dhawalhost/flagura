package openfeature

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	flagura "github.com/dhawalhost/flagura/sdks/go"
	of "github.com/open-feature/go-sdk/openfeature"
)

// Provider implements the openfeature.FeatureProvider and openfeature.EventHandler interfaces.
type Provider struct {
	client     *flagura.Client
	eventsChan chan of.Event
}

// NewProvider creates a new OpenFeature Provider wrapping a Flagura client.
func NewProvider(c *flagura.Client) *Provider {
	eventsChan := make(chan of.Event, 50)
	p := &Provider{
		client:     c,
		eventsChan: eventsChan,
	}

	if c != nil {
		c.RegisterUpdateListener(func(flags map[string]flagura.FeatureFlag, changedKeys []string) {
			select {
			case p.eventsChan <- of.Event{
				ProviderName: "flagura-go-provider",
				EventType:    of.ProviderConfigChange,
				ProviderEventDetails: of.ProviderEventDetails{
					Message:     "Flag configurations synchronized from Flagura control plane",
					FlagChanges: changedKeys,
				},
			}:
			default:
			}
		})
	}

	return p
}

// EventChannel exposes the event channel for the OpenFeature SDK eventing bus.
func (p *Provider) EventChannel() <-chan of.Event {
	return p.eventsChan
}

// Init initializes the provider and emits a PROVIDER_READY event.
func (p *Provider) Init(e of.EvaluationContext) error {
	select {
	case p.eventsChan <- of.Event{
		ProviderName: "flagura-go-provider",
		EventType:    of.ProviderReady,
		ProviderEventDetails: of.ProviderEventDetails{
			Message: "Flagura provider initialized successfully",
		},
	}:
	default:
	}
	return nil
}

// Shutdown gracefully terminates the provider.
func (p *Provider) Shutdown() {
	if p.client != nil {
		p.client.Close()
	}
}

// Metadata returns the metadata of the provider.
func (p *Provider) Metadata() of.Metadata {
	return of.Metadata{
		Name: "flagura-go-provider",
	}
}

// Hooks returns hooks associated with the provider.
func (p *Provider) Hooks() []of.Hook {
	return []of.Hook{}
}

// BooleanEvaluation evaluates a boolean feature flag.
func (p *Provider) BooleanEvaluation(ctx context.Context, flag string, defaultValue bool, flatCtx of.FlattenedContext) of.BoolResolutionDetail {
	evalCtx := extractContext(flatCtx)
	res, err := p.client.Evaluate(ctx, flag, evalCtx)
	if err != nil {
		return of.BoolResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: mapEvaluationError(err, res.Reason),
				Reason:          of.ErrorReason,
			},
		}
	}

	return of.BoolResolutionDetail{
		Value: res.Enabled,
		ProviderResolutionDetail: of.ProviderResolutionDetail{
			Variant: res.Variant,
			Reason:  mapReason(res.Reason),
		},
	}
}

// StringEvaluation evaluates a string feature flag.
func (p *Provider) StringEvaluation(ctx context.Context, flag string, defaultValue string, flatCtx of.FlattenedContext) of.StringResolutionDetail {
	evalCtx := extractContext(flatCtx)
	res, err := p.client.Evaluate(ctx, flag, evalCtx)
	if err != nil {
		return of.StringResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: mapEvaluationError(err, res.Reason),
				Reason:          of.ErrorReason,
			},
		}
	}

	val := defaultValue
	if res.Variant != "" {
		val = res.Variant
	}
	if strVal, ok := res.Value.(string); ok && strVal != "" {
		val = strVal
	}

	return of.StringResolutionDetail{
		Value: val,
		ProviderResolutionDetail: of.ProviderResolutionDetail{
			Variant: res.Variant,
			Reason:  mapReason(res.Reason),
		},
	}
}

// FloatEvaluation evaluates a float feature flag.
func (p *Provider) FloatEvaluation(ctx context.Context, flag string, defaultValue float64, flatCtx of.FlattenedContext) of.FloatResolutionDetail {
	evalCtx := extractContext(flatCtx)
	res, err := p.client.Evaluate(ctx, flag, evalCtx)
	if err != nil {
		return of.FloatResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: mapEvaluationError(err, res.Reason),
				Reason:          of.ErrorReason,
			},
		}
	}

	val := defaultValue
	switch v := res.Value.(type) {
	case float64:
		val = v
	case int:
		val = float64(v)
	case int64:
		val = float64(v)
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			val = parsed
		}
	}

	return of.FloatResolutionDetail{
		Value: val,
		ProviderResolutionDetail: of.ProviderResolutionDetail{
			Variant: res.Variant,
			Reason:  mapReason(res.Reason),
		},
	}
}

// IntEvaluation evaluates an integer feature flag.
func (p *Provider) IntEvaluation(ctx context.Context, flag string, defaultValue int64, flatCtx of.FlattenedContext) of.IntResolutionDetail {
	evalCtx := extractContext(flatCtx)
	res, err := p.client.Evaluate(ctx, flag, evalCtx)
	if err != nil {
		return of.IntResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: mapEvaluationError(err, res.Reason),
				Reason:          of.ErrorReason,
			},
		}
	}

	val := defaultValue
	switch v := res.Value.(type) {
	case int64:
		val = v
	case int:
		val = int64(v)
	case float64:
		val = int64(v)
	case string:
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			val = parsed
		}
	}

	return of.IntResolutionDetail{
		Value: val,
		ProviderResolutionDetail: of.ProviderResolutionDetail{
			Variant: res.Variant,
			Reason:  mapReason(res.Reason),
		},
	}
}

// ObjectEvaluation evaluates an arbitrary structured JSON object feature flag.
func (p *Provider) ObjectEvaluation(ctx context.Context, flag string, defaultValue interface{}, flatCtx of.FlattenedContext) of.InterfaceResolutionDetail {
	evalCtx := extractContext(flatCtx)
	res, err := p.client.Evaluate(ctx, flag, evalCtx)
	if err != nil {
		return of.InterfaceResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: mapEvaluationError(err, res.Reason),
				Reason:          of.ErrorReason,
			},
		}
	}

	val := res.Value
	if val == nil {
		val = defaultValue
	}

	return of.InterfaceResolutionDetail{
		Value: val,
		ProviderResolutionDetail: of.ProviderResolutionDetail{
			Variant: res.Variant,
			Reason:  mapReason(res.Reason),
		},
	}
}

func extractContext(flatCtx of.FlattenedContext) flagura.Context {
	evalCtx := flagura.Context{
		Custom: make(map[string]interface{}),
	}

	for k, v := range flatCtx {
		switch strings.ToLower(k) {
		case "targetingkey", "user_id", "userid":
			evalCtx.UserID = fmt.Sprintf("%v", v)
		case "email":
			evalCtx.Email = fmt.Sprintf("%v", v)
		case "country":
			evalCtx.Country = fmt.Sprintf("%v", v)
		case "role":
			evalCtx.Role = fmt.Sprintf("%v", v)
		case "tier":
			evalCtx.Tier = fmt.Sprintf("%v", v)
		case "environment":
			evalCtx.Environment = flagura.Environment(fmt.Sprintf("%v", v))
		default:
			evalCtx.Custom[k] = v
		}
	}

	return evalCtx
}

func mapReason(internalReason string) of.Reason {
	switch {
	case strings.Contains(internalReason, "FLAG_NOT_FOUND"):
		return of.ErrorReason
	case strings.Contains(internalReason, "RULE_MATCH"):
		return of.TargetingMatchReason
	case strings.Contains(internalReason, "PERCENTAGE_ROLLOUT"), strings.Contains(internalReason, "MULTIVARIATE"):
		return of.SplitReason
	case strings.Contains(internalReason, "DISABLED"):
		return of.DisabledReason
	default:
		return of.DefaultReason
	}
}

func mapEvaluationError(err error, reason string) of.ResolutionError {
	if strings.Contains(reason, "FLAG_NOT_FOUND") || strings.Contains(err.Error(), "not found") {
		return of.NewFlagNotFoundResolutionError(err.Error())
	}
	if strings.Contains(reason, "TYPE_MISMATCH") {
		return of.NewTypeMismatchResolutionError(err.Error())
	}
	return of.NewGeneralResolutionError(err.Error())
}
