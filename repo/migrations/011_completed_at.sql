-- migration 011: add completed_at to orders table
--
-- The 14-day return window must be anchored to the exact moment an order was
-- marked "Completed", not to updated_at which resets on every status change.
-- This column is set by the Transition trigger when toStatus = 'completed'
-- and remains NULL for orders that have not yet reached that state.

ALTER TABLE orders ADD COLUMN completed_at TEXT;
