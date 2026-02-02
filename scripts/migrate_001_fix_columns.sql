-- Migration 001: Fix column sizes and add constraints
-- Run this in Supabase SQL Editor

-- 1. The market_regime column was storing Claude's market assessment which can be long
-- Change to TEXT for flexibility
ALTER TABLE signals 
    ALTER COLUMN market_regime TYPE TEXT;

-- 2. Add unique constraint on holdings ticker for upsert operations
-- First remove duplicates if any exist
DELETE FROM holdings a USING holdings b
WHERE a.id < b.id AND a.ticker = b.ticker;

-- Then add the unique constraint
ALTER TABLE holdings 
    ADD CONSTRAINT holdings_ticker_unique UNIQUE (ticker);

-- Verify the changes
SELECT column_name, data_type, character_maximum_length 
FROM information_schema.columns 
WHERE table_name = 'signals';

SELECT constraint_name, constraint_type 
FROM information_schema.table_constraints 
WHERE table_name = 'holdings';
