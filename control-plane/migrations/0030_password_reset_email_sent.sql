ALTER TABLE password_reset_tokens
  ADD COLUMN IF NOT EXISTS email_sent_at timestamptz;
