use flagura::{EvaluationContext, FlaguraClient};
use std::error::Error;

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    let endpoint = std::env::var("FLAGURA_ENDPOINT").unwrap_or_else(|_| "http://localhost:3000".to_string());
    let api_key = std::env::var("FLAGURA_API_KEY").unwrap_or_else(|_| "flg_live_demo_key_example".to_string());

    println!("🚀 Flagura Rust Integration Example");
    println!("   Endpoint: {}\n", endpoint);

    // Initialize async Rust client
    let client = FlaguraClient::new(&endpoint)
        .with_api_key(&api_key);

    let ctx = EvaluationContext::new("usr_alex_42")
        .with_email("alex@company.com")
        .with_country("US")
        .with_tier("pro");

    // Fast boolean evaluation
    if client.is_enabled("ai-smart-search", &ctx).await {
        println!("✨ Feature flag 'ai-smart-search' is ENABLED for usr_alex_42");
    } else {
        println!("🔒 Feature flag 'ai-smart-search' is DISABLED for usr_alex_42");
    }

    // Detailed evaluation with variant and execution latency
    let res = client.evaluate("ai-smart-search", &ctx).await?;
    println!(
        "Evaluation Details: Flag={}, Enabled={}, Variant={}, Reason={}, Latency={}us",
        res.flag_key, res.enabled, res.variant, res.reason, res.latency_us
    );

    println!("\n✅ Rust evaluation completed successfully.");
    Ok(())
}
