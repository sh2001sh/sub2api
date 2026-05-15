INSERT INTO settings (key, value, updated_at)
VALUES
    ('MIN_RECHARGE_AMOUNT', '10.00', NOW()),
    ('BALANCE_RECHARGE_MULTIPLIER', '1.6667', NOW())
ON CONFLICT (key) DO UPDATE
SET
    value = EXCLUDED.value,
    updated_at = EXCLUDED.updated_at;

WITH desired_groups AS (
    SELECT *
    FROM (
        VALUES
            ('日卡 50 美元', '10 元开通，24 小时内可用 50 美元额度', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', 50::DECIMAL(20,8), NULL::DECIMAL(20,8), NULL::DECIMAL(20,8), 1, '["claude","gemini_text","gemini_image"]'::jsonb, 10),
            ('日卡 100 美元', '20 元开通，24 小时内可用 100 美元额度', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', 100::DECIMAL(20,8), NULL::DECIMAL(20,8), NULL::DECIMAL(20,8), 1, '["claude","gemini_text","gemini_image"]'::jsonb, 20),
            ('Lite 月卡', '轻度月卡，每周 50 美元，月总额度 200 美元', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', NULL::DECIMAL(20,8), 50::DECIMAL(20,8), 200::DECIMAL(20,8), 30, '["claude","gemini_text","gemini_image"]'::jsonb, 30),
            ('Standard 月卡', '主流月卡，每周 100 美元，月总额度 400 美元', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', NULL::DECIMAL(20,8), 100::DECIMAL(20,8), 400::DECIMAL(20,8), 30, '["claude","gemini_text","gemini_image"]'::jsonb, 40),
            ('Pro 月卡', '重度月卡，每周 250 美元，月总额度 1000 美元', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', NULL::DECIMAL(20,8), 250::DECIMAL(20,8), 1000::DECIMAL(20,8), 30, '["claude","gemini_text","gemini_image"]'::jsonb, 50),
            ('Ultra 月卡', '旗舰月卡，每周 750 美元，月总额度 3000 美元', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', NULL::DECIMAL(20,8), 750::DECIMAL(20,8), 3000::DECIMAL(20,8), 30, '["claude","gemini_text","gemini_image"]'::jsonb, 60)
    ) AS v(name, description, rate_multiplier, platform, subscription_type, daily_limit_usd, weekly_limit_usd, monthly_limit_usd, default_validity_days, supported_model_scopes, sort_order)
)
UPDATE groups AS g
SET
    description = d.description,
    rate_multiplier = d.rate_multiplier,
    is_exclusive = FALSE,
    status = 'active',
    platform = d.platform,
    subscription_type = d.subscription_type,
    daily_limit_usd = d.daily_limit_usd,
    weekly_limit_usd = d.weekly_limit_usd,
    monthly_limit_usd = d.monthly_limit_usd,
    default_validity_days = d.default_validity_days,
    supported_model_scopes = d.supported_model_scopes,
    sort_order = d.sort_order,
    updated_at = NOW()
FROM desired_groups AS d
WHERE g.name = d.name
  AND g.deleted_at IS NULL;

WITH desired_groups AS (
    SELECT *
    FROM (
        VALUES
            ('日卡 50 美元', '10 元开通，24 小时内可用 50 美元额度', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', 50::DECIMAL(20,8), NULL::DECIMAL(20,8), NULL::DECIMAL(20,8), 1, '["claude","gemini_text","gemini_image"]'::jsonb, 10),
            ('日卡 100 美元', '20 元开通，24 小时内可用 100 美元额度', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', 100::DECIMAL(20,8), NULL::DECIMAL(20,8), NULL::DECIMAL(20,8), 1, '["claude","gemini_text","gemini_image"]'::jsonb, 20),
            ('Lite 月卡', '轻度月卡，每周 50 美元，月总额度 200 美元', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', NULL::DECIMAL(20,8), 50::DECIMAL(20,8), 200::DECIMAL(20,8), 30, '["claude","gemini_text","gemini_image"]'::jsonb, 30),
            ('Standard 月卡', '主流月卡，每周 100 美元，月总额度 400 美元', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', NULL::DECIMAL(20,8), 100::DECIMAL(20,8), 400::DECIMAL(20,8), 30, '["claude","gemini_text","gemini_image"]'::jsonb, 40),
            ('Pro 月卡', '重度月卡，每周 250 美元，月总额度 1000 美元', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', NULL::DECIMAL(20,8), 250::DECIMAL(20,8), 1000::DECIMAL(20,8), 30, '["claude","gemini_text","gemini_image"]'::jsonb, 50),
            ('Ultra 月卡', '旗舰月卡，每周 750 美元，月总额度 3000 美元', 1.0::DECIMAL(10,4), 'anthropic', 'subscription', NULL::DECIMAL(20,8), 750::DECIMAL(20,8), 3000::DECIMAL(20,8), 30, '["claude","gemini_text","gemini_image"]'::jsonb, 60)
    ) AS v(name, description, rate_multiplier, platform, subscription_type, daily_limit_usd, weekly_limit_usd, monthly_limit_usd, default_validity_days, supported_model_scopes, sort_order)
)
INSERT INTO groups (
    name,
    description,
    rate_multiplier,
    is_exclusive,
    status,
    platform,
    subscription_type,
    daily_limit_usd,
    weekly_limit_usd,
    monthly_limit_usd,
    default_validity_days,
    supported_model_scopes,
    sort_order,
    created_at,
    updated_at
)
SELECT
    d.name,
    d.description,
    d.rate_multiplier,
    FALSE,
    'active',
    d.platform,
    d.subscription_type,
    d.daily_limit_usd,
    d.weekly_limit_usd,
    d.monthly_limit_usd,
    d.default_validity_days,
    d.supported_model_scopes,
    d.sort_order,
    NOW(),
    NOW()
FROM desired_groups AS d
WHERE NOT EXISTS (
    SELECT 1
    FROM groups AS g
    WHERE g.name = d.name
      AND g.deleted_at IS NULL
);

WITH desired_plans AS (
    SELECT *
    FROM (
        VALUES
            ('日卡 50 美元', '日卡 50 美元', '10 元开通，24 小时总额度 50 美元', 10.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'days', E'24 小时有效\n总额度 50 美元\n适合试用与短期项目', '日卡 50 美元额度', TRUE, 10),
            ('日卡 100 美元', '日卡 100 美元', '20 元开通，24 小时总额度 100 美元', 20.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'days', E'24 小时有效\n总额度 100 美元\n适合短时高强度使用', '日卡 100 美元额度', TRUE, 20),
            ('Lite 月卡', 'Lite 月卡', '40 元月卡，每周 50 美元，月总额度 200 美元', 40.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'months', E'每周限额 50 美元\n月总额度 200 美元\n适合轻度用户', 'Lite 月卡', TRUE, 30),
            ('Standard 月卡', 'Standard 月卡', '75 元月卡，每周 100 美元，月总额度 400 美元', 75.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'months', E'每周限额 100 美元\n月总额度 400 美元\n适合主流用户', 'Standard 月卡', TRUE, 40),
            ('Pro 月卡', 'Pro 月卡', '140 元月卡，每周 250 美元，月总额度 1000 美元', 140.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'months', E'每周限额 250 美元\n月总额度 1000 美元\n适合重度用户', 'Pro 月卡', TRUE, 50),
            ('Ultra 月卡', 'Ultra 月卡', '300 元月卡，每周 750 美元，月总额度 3000 美元', 300.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'months', E'每周限额 750 美元\n月总额度 3000 美元\n适合超重度用户', 'Ultra 月卡', TRUE, 60)
    ) AS v(name, group_name, description, price, original_price, validity_days, validity_unit, features, product_name, for_sale, sort_order)
)
UPDATE subscription_plans AS p
SET
    group_id = g.id,
    description = d.description,
    price = d.price,
    original_price = d.original_price,
    validity_days = d.validity_days,
    validity_unit = d.validity_unit,
    features = d.features,
    product_name = d.product_name,
    for_sale = d.for_sale,
    sort_order = d.sort_order,
    updated_at = NOW()
FROM desired_plans AS d
JOIN groups AS g
    ON g.name = d.group_name
   AND g.deleted_at IS NULL
WHERE p.name = d.name;

WITH desired_plans AS (
    SELECT *
    FROM (
        VALUES
            ('日卡 50 美元', '日卡 50 美元', '10 元开通，24 小时总额度 50 美元', 10.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'days', E'24 小时有效\n总额度 50 美元\n适合试用与短期项目', '日卡 50 美元额度', TRUE, 10),
            ('日卡 100 美元', '日卡 100 美元', '20 元开通，24 小时总额度 100 美元', 20.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'days', E'24 小时有效\n总额度 100 美元\n适合短时高强度使用', '日卡 100 美元额度', TRUE, 20),
            ('Lite 月卡', 'Lite 月卡', '40 元月卡，每周 50 美元，月总额度 200 美元', 40.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'months', E'每周限额 50 美元\n月总额度 200 美元\n适合轻度用户', 'Lite 月卡', TRUE, 30),
            ('Standard 月卡', 'Standard 月卡', '75 元月卡，每周 100 美元，月总额度 400 美元', 75.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'months', E'每周限额 100 美元\n月总额度 400 美元\n适合主流用户', 'Standard 月卡', TRUE, 40),
            ('Pro 月卡', 'Pro 月卡', '140 元月卡，每周 250 美元，月总额度 1000 美元', 140.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'months', E'每周限额 250 美元\n月总额度 1000 美元\n适合重度用户', 'Pro 月卡', TRUE, 50),
            ('Ultra 月卡', 'Ultra 月卡', '300 元月卡，每周 750 美元，月总额度 3000 美元', 300.00::DECIMAL(20,2), NULL::DECIMAL(20,2), 1, 'months', E'每周限额 750 美元\n月总额度 3000 美元\n适合超重度用户', 'Ultra 月卡', TRUE, 60)
    ) AS v(name, group_name, description, price, original_price, validity_days, validity_unit, features, product_name, for_sale, sort_order)
)
INSERT INTO subscription_plans (
    group_id,
    name,
    description,
    price,
    original_price,
    validity_days,
    validity_unit,
    features,
    product_name,
    for_sale,
    sort_order,
    created_at,
    updated_at
)
SELECT
    g.id,
    d.name,
    d.description,
    d.price,
    d.original_price,
    d.validity_days,
    d.validity_unit,
    d.features,
    d.product_name,
    d.for_sale,
    d.sort_order,
    NOW(),
    NOW()
FROM desired_plans AS d
JOIN groups AS g
    ON g.name = d.group_name
   AND g.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1
    FROM subscription_plans AS p
    WHERE p.name = d.name
);
