// Package openfeature provides an official OpenFeature Go SDK provider for Flagura.
//
// It allows applications using the vendor-neutral OpenFeature SDK to use Flagura's
// high-performance, in-memory evaluation engine as their feature flag backend.
package openfeature

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dhawalhost/flagura/pkg/client"
	"github.com/dhawalhost/flagura/pkg/domain"
	of "github.com/open-feature/go-sdk/openfeature"
)

// Provider implements the openfeature.FeatureProvider and openfeature.EventHandler interfaces.
type Provider struct {
	client     *client.Client
	eventsChan chan of.Event
}

// Option configures the OpenFeature Flagura Provider.
type Option func(*Provider)

// NewProvider creates a new OpenFeature Provider wrapping a Flagura client.
func NewProvider(c *client.Client) *Provider {
	eventsChan := make(chan of.Event, 50)
	p := &Provider{
		client:     c,
		eventsChan: eventsChan,
	}

	if c != nil {
		c.RegisterUpdateListener(func(flags map[string]domain.FeatureFlag, changedKeys []string) {
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
	// Graceful shutdown
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

	if isFlagNotFound(res.Reason) {
		return of.BoolResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: of.NewFlagNotFoundResolutionError(fmt.Sprintf("flag '%s' not found", flag)),
				Reason:          of.ErrorReason,
			},
		}
	}

	if !res.Enabled {
		return of.BoolResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				Reason:  mapReason(res.Reason),
				Variant: res.Variant,
			},
		}
	}

	// Flag is enabled
	val := defaultValue
	if b, ok := res.Value.(bool); ok {
		val = b
	} else if res.Value != nil {
		if str, ok := res.Value.(string); ok {
			if parsed, err := strconv.ParseBool(str); err == nil {
				val = parsed
			}
		}
	} else {
		val = true
	}

	return of.BoolResolutionDetail{
		Value: val,
		ProviderResolutionDetail: of.ProviderResolutionDetail{
			Reason:  mapReason(res.Reason),
			Variant: res.Variant,
		},
	}
}

// StringEvaluation evaluates a string feature flag / multivariate variant key.
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

	if isFlagNotFound(res.Reason) {
		return of.StringResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: of.NewFlagNotFoundResolutionError(fmt.Sprintf("flag '%s' not found", flag)),
				Reason:          of.ErrorReason,
			},
		}
	}

	if !res.Enabled {
		return of.StringResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				Reason:  mapReason(res.Reason),
				Variant: res.Variant,
			},
		}
	}

	val := defaultValue
	if str, ok := res.Value.(string); ok && str != "" {
		val = str
	} else if res.Variant != "" {
		val = res.Variant
	}

	return of.StringResolutionDetail{
		Value: val,
		ProviderResolutionDetail: of.ProviderResolutionDetail{
			Reason:  mapReason(res.Reason),
			Variant: res.Variant,
		},
	}
}

// FloatEvaluation evaluates a numeric float feature flag.
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

	if isFlagNotFound(res.Reason) {
		return of.FloatResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: of.NewFlagNotFoundResolutionError(fmt.Sprintf("flag '%s' not found", flag)),
				Reason:          of.ErrorReason,
			},
		}
	}

	if !res.Enabled {
		return of.FloatResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				Reason:  mapReason(res.Reason),
				Variant: res.Variant,
			},
		}
	}

	val, err := toFloat64(res.Value)
	if err != nil {
		return of.FloatResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: of.NewTypeMismatchResolutionError(fmt.Sprintf("cannot convert value %v to float64", res.Value)),
				Reason:          of.ErrorReason,
			},
		}
	}

	return of.FloatResolutionDetail{
		Value: val,
		ProviderResolutionDetail: of.ProviderResolutionDetail{
			Reason:  mapReason(res.Reason),
			Variant: res.Variant,
		},
	}
}

// IntEvaluation evaluates an integer numeric feature flag.
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

	if isFlagNotFound(res.Reason) {
		return of.IntResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: of.NewFlagNotFoundResolutionError(fmt.Sprintf("flag '%s' not found", flag)),
				Reason:          of.ErrorReason,
			},
		}
	}

	if !res.Enabled {
		return of.IntResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				Reason:  mapReason(res.Reason),
				Variant: res.Variant,
			},
		}
	}

	val, err := toInt64(res.Value)
	if err != nil {
		return of.IntResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: of.NewTypeMismatchResolutionError(fmt.Sprintf("cannot convert value %v to int64", res.Value)),
				Reason:          of.ErrorReason,
			},
		}
	}

	return of.IntResolutionDetail{
		Value: val,
		ProviderResolutionDetail: of.ProviderResolutionDetail{
			Reason:  mapReason(res.Reason),
			Variant: res.Variant,
		},
	}
}

// ObjectEvaluation evaluates a structured JSON or interface{} feature flag.
func (p *Provider) ObjectEvaluation(ctx context.Context, flag string, defaultValue any, flatCtx of.FlattenedContext) of.InterfaceResolutionDetail {
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

	if isFlagNotFound(res.Reason) {
		return of.InterfaceResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				ResolutionError: of.NewFlagNotFoundResolutionError(fmt.Sprintf("flag '%s' not found", flag)),
				Reason:          of.ErrorReason,
			},
		}
	}

	if !res.Enabled {
		return of.InterfaceResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: of.ProviderResolutionDetail{
				Reason:  mapReason(res.Reason),
				Variant: res.Variant,
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
			Reason:  mapReason(res.Reason),
			Variant: res.Variant,
		},
	}
}

func extractContext(flatCtx of.FlattenedContext) client.Context {
	evalCtx := client.Context{
		Custom: make(map[string]interface{}),
	}

	if flatCtx == nil {
		return evalCtx
	}

	for k, v := range flatCtx {
		switch k {
		case of.TargetingKey, "user_id", "userId", "id":
			if str, ok := v.(string); ok {
				evalCtx.UserID = str
			}
		case "email":
			if str, ok := v.(string); ok {
				evalCtx.Email = str
			}
		case "country":
			if str, ok := v.(string); ok {
				evalCtx.Country = str
			}
		case "role":
			if str, ok := v.(string); ok {
				evalCtx.Role = str
			}
		case "tier":
			if str, ok := v.(string); ok {
				evalCtx.Tier = str
			}
		case "environment", "env":
			if str, ok := v.(string); ok {
				evalCtx.Environment = domain.Environment(str)
			}
		default:
			evalCtx.Custom[k] = v
		}
	}

	return evalCtx
}

func isFlagNotFound(r string) bool {
	norm := strings.ToUpper(r)
	return norm == string(domain.ReasonFlagNotFound) || norm == "FLAG_NOT_FOUND"
}

func mapReason(r string) of.Reason {
	norm := strings.ToUpper(r)
	switch norm {
	case string(domain.ReasonKillSwitchDisabled), string(domain.ReasonEnvDisabled), "DISABLED":
		return of.DisabledReason
	case string(domain.ReasonTargetingRuleMatch), "TARGETING_MATCH":
		return of.TargetingMatchReason
	case string(domain.ReasonPercentageBucket), string(domain.ReasonMultivariateBucket), "PERCENTAGE_ROLLOUT_MATCH", "MULTIVARIATE_VARIANT_MATCH", "SPLIT":
		return of.SplitReason
	case string(domain.ReasonDefaultEnabled), string(domain.ReasonDefaultOff), "DEFAULT_VARIANT", "DEFAULT":
		return of.DefaultReason
	default:
		return of.StaticReason
	}
}

func mapEvaluationError(err error, reason string) of.ResolutionError {
	norm := strings.ToUpper(reason)
	if norm == string(domain.ReasonFlagNotFound) || norm == "FLAG_NOT_FOUND" {
		return of.NewFlagNotFoundResolutionError(err.Error())
	}
	return of.NewGeneralResolutionError(err.Error())
}

func toFloat64(v interface{}) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case string:
		return strconv.ParseFloat(n, 64)
	default:
		return 0, fmt.Errorf("invalid float type %T", v)
	}
}

func toInt64(v interface{}) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case int32:
		return int64(n), nil
	case float64:
		return int64(n), nil
	case string:
		return strconv.ParseInt(n, 10, 64)
	default:
		return 0, fmt.Errorf("invalid int type %T", v)
	}
}
