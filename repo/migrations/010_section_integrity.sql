-- 010_section_integrity.sql
--
-- Adds a DB-level trigger to enforce that course_plans.section_id, when
-- non-NULL, references a course_section that belongs to the same course as
-- the plan row itself.  This prevents data corruption where a section from
-- course A is accidentally attached to a plan for course B.

CREATE TRIGGER IF NOT EXISTS trg_course_plans_section_course_match_insert
BEFORE INSERT ON course_plans
WHEN NEW.section_id IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'section_id does not belong to the plan''s course_id')
    WHERE NOT EXISTS (
        SELECT 1 FROM course_sections
        WHERE id = NEW.section_id AND course_id = NEW.course_id
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_course_plans_section_course_match_update
BEFORE UPDATE ON course_plans
WHEN NEW.section_id IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'section_id does not belong to the plan''s course_id')
    WHERE NOT EXISTS (
        SELECT 1 FROM course_sections
        WHERE id = NEW.section_id AND course_id = NEW.course_id
    );
END;
