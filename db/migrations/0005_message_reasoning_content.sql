-- Add reasoning_content column to store model thinking/reasoning tokens
ALTER TABLE messages ADD COLUMN reasoning_content TEXT;
