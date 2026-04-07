export interface BaselineModel {
  model: string;
  count: number;
  mean_output_tokens: number;
  stddev_output_tokens: number;
  mean_latency_ms: number;
  stddev_latency_ms: number;
  is_ready: boolean;
  // Drift — zero/false until the detector has run at least once
  drift_score: number;
  drift_alerted: boolean;
  p_output_tokens: number;
  p_latency_ms: number;
}

export interface BaselinesResponse {
  total_events: number;
  baselines: BaselineModel[];
}
