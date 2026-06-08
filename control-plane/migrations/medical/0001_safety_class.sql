-- Medical migration 0001: IEC 62304 safety class on artifacts.
-- Applied only when DEPLOYMENT_PROFILE=medical.

ALTER TABLE artifacts
    ADD COLUMN IF NOT EXISTS safety_class text
        CHECK (safety_class IN ('ClassA', 'ClassB', 'ClassC'));
