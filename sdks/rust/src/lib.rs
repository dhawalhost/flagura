use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum FlaguraError {
    #[error("HTTP network request failed: {0}")]
    Http(#[from] reqwest::Error),
    #[error("JSON serialization error: {0}")]
    Json(#[from] serde_json::Error),
    #[error("Flagura API returned error status {status}: {message}")]
    Api { status: u16, message: String },
    #[error("Evaluation context error: {0}")]
    Context(String),
}

pub type Result<T> = std::result::Result<T, FlaguraError>;

/// Context attributes used during rule evaluation and rollout bucketing.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct EvaluationContext {
    pub user_id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub email: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub country: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub role: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tier: Option<String>,
    #[serde(default = "default_env")]
    pub environment: String,
    #[serde(default, skip_serializing_if = "HashMap::is_empty")]
    pub custom: HashMap<String, serde_json::Value>,
}

fn default_env() -> String {
    "production".to_string()
}

impl EvaluationContext {
    pub fn new(user_id: impl Into<String>) -> Self {
        Self {
            user_id: user_id.into(),
            environment: "production".to_string(),
            ..Default::default()
        }
    }

    pub fn with_email(mut self, email: impl Into<String>) -> Self {
        self.email = Some(email.into());
        self
    }

    pub fn with_country(mut self, country: impl Into<String>) -> Self {
        self.country = Some(country.into());
        self
    }

    pub fn with_tier(mut self, tier: impl Into<String>) -> Self {
        self.tier = Some(tier.into());
        self
    }

    pub fn with_environment(mut self, env: impl Into<String>) -> Self {
        self.environment = env.into();
        self
    }

    pub fn with_attribute(mut self, key: impl Into<String>, value: serde_json::Value) -> Self {
        self.custom.insert(key.into(), value);
        self
    }
}

/// Evaluation result returned for a specific feature flag evaluation.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EvaluationResult {
    pub flag_key: String,
    pub enabled: bool,
    pub variant: String,
    pub value: serde_json::Value,
    pub reason: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bucket: Option<f64>,
    #[serde(default)]
    pub latency_ns: u64,
    #[serde(default)]
    pub latency_us: f64,
}

#[derive(Serialize)]
struct BatchEvaluateRequest<'a> {
    flags: &'a [&'a str],
    context: &'a EvaluationContext,
}

#[derive(Deserialize)]
struct BatchEvaluateResponse {
    results: HashMap<String, EvaluationResult>,
}

#[derive(Serialize)]
struct TrackEventPayload<'a> {
    event: TrackEvent<'a>,
}

#[derive(Serialize)]
struct TrackEvent<'a> {
    flag_key: &'a str,
    variant: &'a str,
    metric_name: &'a str,
    value: f64,
    user_id: &'a str,
    environment: &'a str,
}

/// Flagura client options.
#[derive(Clone)]
pub struct ClientConfig {
    pub endpoint: String,
    pub api_key: Option<String>,
    pub project_id: Option<String>,
    pub default_environment: String,
    pub timeout: Duration,
}

/// Native Flagura Client for Rust.
#[derive(Clone)]
pub struct FlaguraClient {
    config: Arc<ClientConfig>,
    http: reqwest::Client,
}

impl FlaguraClient {
    /// Creates a new FlaguraClient with the given endpoint.
    pub fn new(endpoint: impl Into<String>) -> Self {
        Self::builder().endpoint(endpoint).build()
    }

    /// Returns a builder to configure client options.
    pub fn builder() -> FlaguraBuilder {
        FlaguraBuilder::default()
    }

    /// Evaluates whether a boolean feature flag is enabled.
    pub async fn is_enabled(&self, flag_key: &str, ctx: &EvaluationContext) -> bool {
        match self.evaluate(flag_key, ctx).await {
            Ok(res) => res.enabled,
            Err(_) => false,
        }
    }

    /// Evaluates a feature flag and returns its complete evaluation metadata.
    pub async fn evaluate(&self, flag_key: &str, ctx: &EvaluationContext) -> Result<EvaluationResult> {
        let results = self.evaluate_batch(&[flag_key], ctx).await?;
        results.get(flag_key).cloned().ok_or_else(|| FlaguraError::Api {
            status: 404,
            message: format!("Flag key '{}' not found in evaluation response", flag_key),
        })
    }

    /// Evaluates multiple feature flags in a single batch request.
    pub async fn evaluate_batch(
        &self,
        flag_keys: &[&str],
        ctx: &EvaluationContext,
    ) -> Result<HashMap<String, EvaluationResult>> {
        let url = format!("{}/api/v1/evaluate", self.config.endpoint.trim_end_matches('/'));
        let mut req = self.http.post(&url).json(&BatchEvaluateRequest {
            flags: flag_keys,
            context: ctx,
        });

        if let Some(ref key) = self.config.api_key {
            req = req.header("Authorization", format!("Bearer {}", key));
        }
        if let Some(ref project_id) = self.config.project_id {
            req = req.header("X-Project-ID", project_id);
        }

        let resp = req.send().await?;
        if !resp.status().is_success() {
            let status = resp.status().as_u16();
            let body = resp.text().await.unwrap_or_default();
            return Err(FlaguraError::Api {
                status,
                message: body,
            });
        }

        let data: BatchEvaluateResponse = resp.json().await?;
        Ok(data.results)
    }

    /// Tracks an A/B experimentation conversion or continuous metric event.
    pub async fn track(
        &self,
        flag_key: &str,
        variant: &str,
        metric_name: &str,
        value: f64,
        user_id: &str,
    ) -> Result<()> {
        let url = format!("{}/api/v1/telemetry/events", self.config.endpoint.trim_end_matches('/'));
        let payload = serde_json::json!({
            "events": [{
                "flag_key": flag_key,
                "project_id": self.config.project_id,
                "variant": variant,
                "metric_name": metric_name,
                "value": value,
                "user_id": user_id,
                "environment": &self.config.default_environment,
            }]
        });

        let mut req = self.http.post(&url).json(&payload);
        if let Some(ref key) = self.config.api_key {
            req = req.header("Authorization", format!("Bearer {}", key));
        }
        if let Some(ref project_id) = self.config.project_id {
            req = req.header("X-Project-ID", project_id);
        }

        let resp = req.send().await?;
        if !resp.status().is_success() {
            let status = resp.status().as_u16();
            let body = resp.text().await.unwrap_or_default();
            return Err(FlaguraError::Api {
                status,
                message: body,
            });
        }

        Ok(())
    }
}

/// Builder for constructing a `FlaguraClient`.
#[derive(Default)]
pub struct FlaguraBuilder {
    endpoint: Option<String>,
    api_key: Option<String>,
    project_id: Option<String>,
    environment: Option<String>,
    timeout: Option<Duration>,
}

impl FlaguraBuilder {
    pub fn endpoint(mut self, endpoint: impl Into<String>) -> Self {
        self.endpoint = Some(endpoint.into());
        self
    }

    pub fn api_key(mut self, key: impl Into<String>) -> Self {
        self.api_key = Some(key.into());
        self
    }

    pub fn project_id(mut self, project_id: impl Into<String>) -> Self {
        self.project_id = Some(project_id.into());
        self
    }

    pub fn environment(mut self, env: impl Into<String>) -> Self {
        self.environment = Some(env.into());
        self
    }

    pub fn timeout(mut self, timeout: Duration) -> Self {
        self.timeout = Some(timeout);
        self
    }

    pub fn build(self) -> FlaguraClient {
        let raw_endpoint = self
            .endpoint
            .or_else(|| std::env::var("FLAGURA_ENDPOINT").ok())
            .unwrap_or_else(|| "https://flagura.dev".to_string());
        let trimmed = raw_endpoint.trim();
        let mut endpoint = trimmed.to_string();
        if !endpoint.starts_with("http://") && !endpoint.starts_with("https://") {
            if endpoint.starts_with("localhost") || endpoint.starts_with("127.0.0.1") {
                endpoint = format!("http://{}", endpoint);
            } else {
                endpoint = format!("https://{}", endpoint);
            }
        }
        let endpoint = endpoint.trim_end_matches('/').to_string();

        let config = ClientConfig {
            endpoint,
            api_key: self.api_key,
            project_id: self.project_id,
            default_environment: self.environment.unwrap_or_else(|| "production".to_string()),
            timeout: self.timeout.unwrap_or(Duration::from_millis(500)),
        };

        let http = reqwest::Client::builder()
            .timeout(config.timeout)
            .build()
            .unwrap_or_default();

        FlaguraClient {
            config: Arc::new(config),
            http,
        }
    }
}
