-- Add BYOK inference cost and throughput metrics for per-message usage reporting.
ALTER TABLE messages ADD COLUMN byok_inference_cost_microusd INTEGER;
ALTER TABLE messages ADD COLUMN tokens_per_second REAL;
