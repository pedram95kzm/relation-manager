export type Locale = "en" | "fa";
export type Module = "self" | "compatibility" | "conflict";
export type Text = { en: string; fa: string };
export type User = {
  id: string;
  email: string;
  name: string;
  role: "user" | "admin";
  age: number | null;
  gender: string;
  disabled: boolean;
  daily_limit?: number;
  created_at: string;
};
export type Session = { access_token: string; expires_in: number; user: User };
export type Page<T> = {
  items: T[];
  total: number;
  page: number;
  page_size: number;
};
export type Answers = Record<string, Record<string, number | null>>;
export type AnalysisContext = Record<string, string | boolean>;
export type Definition = {
  module: Module;
  version: string;
  scoring_version: string;
  title: Text;
  description: Text;
  unknown_label: Text;
  dimensions: { id: string; label: Text }[];
  questions: {
    id: string;
    group: string;
    text: Text;
    required: boolean;
    allow_unknown: boolean;
    options: { value: number; label: Text }[];
  }[];
  context_fields: {
    id: string;
    text: Text;
    type: string;
    required: boolean;
    options?: { value: string; label: Text }[];
    max_length?: number;
  }[];
};
export type Dimension = {
  id: string;
  label: Text;
  score: number | null;
  band: string | null;
  answered: number;
  total: number;
  unknown: number;
  omitted: number;
  completeness: number;
};
export type Scores = {
  status: string;
  respondents: Record<
    string,
    {
      dimensions: Dimension[];
      answered: number;
      total: number;
      unknown: number;
      omitted: number;
      numeric_completeness: number;
    }
  >;
  compatibility?: {
    index: number | null;
    status: string;
    coverage: number;
    included_dimensions: string[];
    omitted_dimensions: string[];
    formula: string;
  };
};
export type Report = {
  kind: "ai" | "deterministic" | "example";
  title: string;
  summary: string;
  sections?: { id: string; title: string; body: string }[];
  recommendations?: string[];
  limitations: string[];
  references?: string[];
  safety?: { level: string; message: string; support: string[] };
};
export type Analysis = {
  id: string;
  module: Module;
  locale: Locale;
  questionnaire_version: string;
  scoring_version: string;
  revision: number;
  status: string;
  answers: Answers;
  context: AnalysisContext;
  scores: Scores;
  report: Report | null;
  failure_code?: string;
  provider: string;
  model: string;
  created_at: string;
  updated_at: string;
};
export type Usage = {
  day: string;
  user_used: number;
  user_limit: number;
  global_used: number;
  global_limit: number;
  report_available: boolean;
  provider?: string;
  is_example?: boolean;
};
export type Provider = {
  configured: boolean;
  provider: string;
  model: string;
  has_key: boolean;
  max_tokens: number;
  timeout_seconds: number;
  retries: number;
  active: boolean;
  is_example?: boolean;
};
