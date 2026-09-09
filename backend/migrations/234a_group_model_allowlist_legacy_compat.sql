-- 234a: keep the pre-0.2.4 groups column available during an application
-- rollback.  Migration 235 renamed models_list_config to model_allowlist;
-- older binaries still select and write the former column.  This migration
-- runs before 235 (and is also safe to apply after 235 was already recorded),
-- so a failed later migration never leaves the old binary without its column.
--
-- Both columns are deliberately retained for the compatibility window.  The
-- trigger keeps them equal when either the old or new application writes, and
-- rejects an ambiguous direct write instead of silently discarding data.

DO $$
DECLARE
    groups_rel regclass := to_regclass('groups');
    has_new boolean;
    has_old boolean;
    conflicting_rows bigint;
    unresolved_rows bigint;
BEGIN
    IF groups_rel IS NULL THEN
        RAISE EXCEPTION 'groups table is required for model allowlist compatibility';
    END IF;

    SELECT EXISTS (
        SELECT 1
          FROM pg_attribute
         WHERE attrelid = groups_rel
           AND attname = 'model_allowlist'
           AND NOT attisdropped
    ) INTO has_new;
    SELECT EXISTS (
        SELECT 1
          FROM pg_attribute
         WHERE attrelid = groups_rel
           AND attname = 'models_list_config'
           AND NOT attisdropped
    ) INTO has_old;

    IF NOT has_new THEN
        EXECUTE format(
            'ALTER TABLE %s ADD COLUMN model_allowlist JSONB NOT NULL DEFAULT ''{}''::jsonb',
            groups_rel
        );
    END IF;
    IF NOT has_old THEN
        EXECUTE format(
            'ALTER TABLE %s ADD COLUMN models_list_config JSONB NOT NULL DEFAULT ''{}''::jsonb',
            groups_rel
        );
    END IF;

    -- A pre-existing dual-column database must not lose a non-empty value from
    -- either side.  Empty defaults are treated as an omitted value and are
    -- backfilled from the populated side.
    EXECUTE format(
        'SELECT count(*) FROM %s
          WHERE COALESCE(model_allowlist, ''{}''::jsonb) IS DISTINCT FROM
                COALESCE(models_list_config, ''{}''::jsonb)
            AND COALESCE(model_allowlist, ''{}''::jsonb) <> ''{}''::jsonb
            AND COALESCE(models_list_config, ''{}''::jsonb) <> ''{}''::jsonb',
        groups_rel
    ) INTO conflicting_rows;
    IF conflicting_rows > 0 THEN
        RAISE EXCEPTION
            'groups model allowlist compatibility found % rows with conflicting non-empty values',
            conflicting_rows;
    END IF;

    EXECUTE format(
        'UPDATE %s
            SET model_allowlist = models_list_config
          WHERE (model_allowlist IS NULL OR model_allowlist = ''{}''::jsonb)
            AND models_list_config IS NOT NULL
            AND models_list_config <> ''{}''::jsonb',
        groups_rel
    );
    EXECUTE format(
        'UPDATE %s
            SET models_list_config = model_allowlist
          WHERE (models_list_config IS NULL OR models_list_config = ''{}''::jsonb)
            AND model_allowlist IS NOT NULL
            AND model_allowlist <> ''{}''::jsonb',
        groups_rel
    );

    EXECUTE format(
        'UPDATE %s
            SET model_allowlist = ''{}''::jsonb
          WHERE model_allowlist IS NULL',
        groups_rel
    );
    EXECUTE format(
        'UPDATE %s
            SET models_list_config = ''{}''::jsonb
          WHERE models_list_config IS NULL',
        groups_rel
    );

    -- Any value that was neither an empty default nor a matching non-empty
    -- value is ambiguous (for example JSONB ``null``).  Do not install the
    -- trigger on a partially synchronized table; fail the transaction so a
    -- caller cannot accidentally run old and new binaries against divergent
    -- data.
    EXECUTE format(
        'SELECT count(*) FROM %s
          WHERE model_allowlist IS DISTINCT FROM models_list_config',
        groups_rel
    ) INTO unresolved_rows;
    IF unresolved_rows > 0 THEN
        RAISE EXCEPTION
            'groups model allowlist compatibility left % rows with unsynchronized values',
            unresolved_rows;
    END IF;

    EXECUTE format(
        'ALTER TABLE %s
            ALTER COLUMN model_allowlist SET DEFAULT ''{}''::jsonb,
            ALTER COLUMN model_allowlist SET NOT NULL,
            ALTER COLUMN models_list_config SET DEFAULT ''{}''::jsonb,
            ALTER COLUMN models_list_config SET NOT NULL',
        groups_rel
    );
END
$$;

CREATE OR REPLACE FUNCTION public.sub2api_sync_group_model_allowlist_compat()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.model_allowlist IS NULL THEN
            NEW.model_allowlist := '{}'::jsonb;
        END IF;
        IF NEW.models_list_config IS NULL THEN
            NEW.models_list_config := '{}'::jsonb;
        END IF;
        IF NEW.model_allowlist = '{}'::jsonb
           AND NEW.models_list_config <> '{}'::jsonb THEN
            NEW.model_allowlist := NEW.models_list_config;
        ELSIF NEW.models_list_config = '{}'::jsonb
              AND NEW.model_allowlist <> '{}'::jsonb THEN
            NEW.models_list_config := NEW.model_allowlist;
        ELSIF NEW.model_allowlist IS DISTINCT FROM NEW.models_list_config THEN
            RAISE EXCEPTION
                'groups model allowlist columns must agree on insert';
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.model_allowlist IS DISTINCT FROM OLD.model_allowlist
       AND NEW.models_list_config IS NOT DISTINCT FROM OLD.models_list_config THEN
        NEW.models_list_config := NEW.model_allowlist;
    ELSIF NEW.models_list_config IS DISTINCT FROM OLD.models_list_config
          AND NEW.model_allowlist IS NOT DISTINCT FROM OLD.model_allowlist THEN
        NEW.model_allowlist := NEW.models_list_config;
    ELSIF NEW.model_allowlist IS DISTINCT FROM NEW.models_list_config THEN
        RAISE EXCEPTION
            'groups model allowlist columns must agree on update';
    END IF;
    RETURN NEW;
END
$$;

DO $$
DECLARE
    groups_rel regclass := to_regclass('groups');
    trigger_function regproc := 'public.sub2api_sync_group_model_allowlist_compat'::regproc;
    existing_function regproc;
BEGIN
    IF groups_rel IS NULL THEN
        RAISE EXCEPTION 'groups table is required for model allowlist compatibility trigger';
    END IF;

    SELECT t.tgfoid::regproc
      INTO existing_function
      FROM pg_trigger t
     WHERE t.tgrelid = groups_rel
       AND t.tgname = 'sub2api_group_model_allowlist_compat_trg'
       AND NOT t.tgisinternal;

    IF existing_function IS NULL THEN
        EXECUTE format(
            'CREATE TRIGGER sub2api_group_model_allowlist_compat_trg
               BEFORE INSERT OR UPDATE OF model_allowlist, models_list_config
               ON %s
               FOR EACH ROW
               EXECUTE FUNCTION public.sub2api_sync_group_model_allowlist_compat()',
            groups_rel
        );
    ELSIF existing_function <> trigger_function THEN
        RAISE EXCEPTION
            'groups model allowlist compatibility trigger is owned by unexpected function %',
            existing_function;
    END IF;
END
$$;

COMMENT ON FUNCTION public.sub2api_sync_group_model_allowlist_compat() IS
    'Keeps pre-0.2.4 models_list_config and model_allowlist synchronized for same-database rollback';
