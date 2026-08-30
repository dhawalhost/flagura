# Flagura Rust SDK (`flagura-rs`)

Official high-performance Rust client SDK for the [Flagura](https://github.com/dhawalhost/flagura) Feature Flag & Experimentation Engine.

## Installation

Add `flagura` to your `Cargo.toml`:

```toml
[dependencies]
flagura = "1.3.0"
tokio = { version = "1", features = ["full"] }
```

## Quickstart

```rust
use flagura::{FlaguraClient, EvaluationContext};
use std::time::Duration;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // 1. Initialize client
    let client = FlaguraClient::builder()
        .endpoint("http://localhost:3000")
        .api_key("flg_live_secret_key")
        .environment("production")
        .timeout(Duration::from_millis(250))
        .build();

    // 2. Build user evaluation context
    let ctx = EvaluationContext::new("usr_alex_42")
        .with_email("alex@company.com")
        .with_country("US")
        .with_tier("enterprise");

    // 3. Fast boolean evaluation
    if client.is_enabled("ai-smart-search", &ctx).await {
        println!("✨ AI Smart Search is active for this user!");
    }

    // 4. Multivariate or detailed evaluation
    let res = client.evaluate("checkout-v2", &ctx).await?;
    println!("Assigned variant: {} (reason: {})", res.variant, res.reason);

    // 5. Track A/B Experiment conversions
    client.track("checkout-v2", &res.variant, "purchase_completed", 49.99, "usr_alex_42").await?;

    Ok(())
}
```

## Features

- **Blazing-Fast Async Evaluation**: Built on Tokio and Reqwest with sub-millisecond evaluation latency.
- **Contextual Rule Targeting**: Match users on ID, email, country, role, tier, or arbitrary custom attributes.
- **A/B Experiment Conversion Tracking**: Seamless `.track()` ingestion directly linked to the statistical engine.
- **Circuit Breaker & Fallback Resilience**: Safe default handling to ensure zero runtime panics in case of network partitions.
