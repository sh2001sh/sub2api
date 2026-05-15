package repository

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const cpaPricingCatalogSeedKey = "cpa_pricing_catalog_seeded_v1"

type cpaPricingGroupSeed struct {
	Name                string
	Description         string
	Platform            string
	SubscriptionType    string
	DailyLimitUSD       *float64
	WeeklyLimitUSD      *float64
	MonthlyLimitUSD     *float64
	DefaultValidityDays int
	SupportedScopes     []string
	SortOrder           int
}

type cpaPricingPlanSeed struct {
	Name         string
	GroupName    string
	Description  string
	Price        float64
	ValidityDays int
	ValidityUnit string
	Features     string
	ProductName  string
	SortOrder    int
}

func ensureCPAPricingCatalog(ctx context.Context, client *dbent.Client) error {
	if client == nil {
		return fmt.Errorf("nil ent client")
	}

	seeded, err := client.Setting.Query().Where(setting.KeyEQ(cpaPricingCatalogSeedKey)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check cpa pricing seed marker: %w", err)
	}
	if seeded {
		return nil
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin cpa pricing seed tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := upsertCPAPricingSettings(ctx, tx.Client()); err != nil {
		return err
	}

	groupIDs, err := upsertCPAPricingGroups(ctx, tx.Client())
	if err != nil {
		return err
	}

	if err := upsertCPAPricingPlans(ctx, tx.Client(), groupIDs); err != nil {
		return err
	}

	now := time.Now()
	if err := tx.Setting.Create().
		SetKey(cpaPricingCatalogSeedKey).
		SetValue(now.Format(time.RFC3339)).
		SetUpdatedAt(now).
		OnConflictColumns(setting.FieldKey).
		UpdateNewValues().
		Exec(ctx); err != nil {
		return fmt.Errorf("persist cpa pricing seed marker: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit cpa pricing seed tx: %w", err)
	}
	return nil
}

func upsertCPAPricingSettings(ctx context.Context, client *dbent.Client) error {
	now := time.Now()
	return client.Setting.CreateBulk(
		client.Setting.Create().SetKey(service.SettingMinRechargeAmount).SetValue("10.00").SetUpdatedAt(now),
		client.Setting.Create().SetKey(service.SettingBalanceRechargeMult).SetValue("1.6667").SetUpdatedAt(now),
	).OnConflictColumns(setting.FieldKey).UpdateNewValues().Exec(ctx)
}

func upsertCPAPricingGroups(ctx context.Context, client *dbent.Client) (map[string]int64, error) {
	seeds := []cpaPricingGroupSeed{
		{
			Name:                "日卡 50 美元",
			Description:         "10 元开通，24 小时内可用 50 美元额度",
			Platform:            service.PlatformAnthropic,
			SubscriptionType:    service.SubscriptionTypeSubscription,
			DailyLimitUSD:       float64Ptr(50),
			DefaultValidityDays: 1,
			SupportedScopes:     []string{"claude", "gemini_text", "gemini_image"},
			SortOrder:           10,
		},
		{
			Name:                "日卡 100 美元",
			Description:         "20 元开通，24 小时内可用 100 美元额度",
			Platform:            service.PlatformAnthropic,
			SubscriptionType:    service.SubscriptionTypeSubscription,
			DailyLimitUSD:       float64Ptr(100),
			DefaultValidityDays: 1,
			SupportedScopes:     []string{"claude", "gemini_text", "gemini_image"},
			SortOrder:           20,
		},
		{
			Name:                "Lite 月卡",
			Description:         "轻度月卡，每周 50 美元，月总额度 200 美元",
			Platform:            service.PlatformAnthropic,
			SubscriptionType:    service.SubscriptionTypeSubscription,
			WeeklyLimitUSD:      float64Ptr(50),
			MonthlyLimitUSD:     float64Ptr(200),
			DefaultValidityDays: 30,
			SupportedScopes:     []string{"claude", "gemini_text", "gemini_image"},
			SortOrder:           30,
		},
		{
			Name:                "Standard 月卡",
			Description:         "主流月卡，每周 100 美元，月总额度 400 美元",
			Platform:            service.PlatformAnthropic,
			SubscriptionType:    service.SubscriptionTypeSubscription,
			WeeklyLimitUSD:      float64Ptr(100),
			MonthlyLimitUSD:     float64Ptr(400),
			DefaultValidityDays: 30,
			SupportedScopes:     []string{"claude", "gemini_text", "gemini_image"},
			SortOrder:           40,
		},
		{
			Name:                "Pro 月卡",
			Description:         "重度月卡，每周 250 美元，月总额度 1000 美元",
			Platform:            service.PlatformAnthropic,
			SubscriptionType:    service.SubscriptionTypeSubscription,
			WeeklyLimitUSD:      float64Ptr(250),
			MonthlyLimitUSD:     float64Ptr(1000),
			DefaultValidityDays: 30,
			SupportedScopes:     []string{"claude", "gemini_text", "gemini_image"},
			SortOrder:           50,
		},
		{
			Name:                "Ultra 月卡",
			Description:         "旗舰月卡，每周 750 美元，月总额度 3000 美元",
			Platform:            service.PlatformAnthropic,
			SubscriptionType:    service.SubscriptionTypeSubscription,
			WeeklyLimitUSD:      float64Ptr(750),
			MonthlyLimitUSD:     float64Ptr(3000),
			DefaultValidityDays: 30,
			SupportedScopes:     []string{"claude", "gemini_text", "gemini_image"},
			SortOrder:           60,
		},
	}

	groupIDs := make(map[string]int64, len(seeds))
	for _, seed := range seeds {
		existing, err := client.Group.Query().
			Where(group.NameEQ(seed.Name), group.DeletedAtIsNil()).
			Only(ctx)
		if err != nil && !dbent.IsNotFound(err) {
			return nil, fmt.Errorf("query group %s: %w", seed.Name, err)
		}

		if existing == nil {
			builder := client.Group.Create().
				SetName(seed.Name).
				SetDescription(seed.Description).
				SetPlatform(seed.Platform).
				SetStatus(service.StatusActive).
				SetSubscriptionType(seed.SubscriptionType).
				SetRateMultiplier(1.0).
				SetIsExclusive(false).
				SetDefaultValidityDays(seed.DefaultValidityDays).
				SetSupportedModelScopes(seed.SupportedScopes).
				SetSortOrder(seed.SortOrder)
			applyCPAPricingGroupLimitsCreate(builder, seed)
			created, createErr := builder.Save(ctx)
			if createErr != nil {
				return nil, fmt.Errorf("create group %s: %w", seed.Name, createErr)
			}
			groupIDs[seed.Name] = created.ID
			continue
		}

		updater := client.Group.UpdateOneID(existing.ID).
			SetDescription(seed.Description).
			SetPlatform(seed.Platform).
			SetStatus(service.StatusActive).
			SetSubscriptionType(seed.SubscriptionType).
			SetRateMultiplier(1.0).
			SetIsExclusive(false).
			SetDefaultValidityDays(seed.DefaultValidityDays).
			SetSupportedModelScopes(seed.SupportedScopes).
			SetSortOrder(seed.SortOrder)
		applyCPAPricingGroupLimitsUpdate(updater, seed)
		updated, updateErr := updater.Save(ctx)
		if updateErr != nil {
			return nil, fmt.Errorf("update group %s: %w", seed.Name, updateErr)
		}
		groupIDs[seed.Name] = updated.ID
	}

	return groupIDs, nil
}

func upsertCPAPricingPlans(ctx context.Context, client *dbent.Client, groupIDs map[string]int64) error {
	seeds := []cpaPricingPlanSeed{
		{
			Name:         "日卡 50 美元",
			GroupName:    "日卡 50 美元",
			Description:  "10 元开通，24 小时总额度 50 美元",
			Price:        10,
			ValidityDays: 1,
			ValidityUnit: "days",
			Features:     "24 小时有效\n总额度 50 美元\n适合试用与短期项目",
			ProductName:  "日卡 50 美元额度",
			SortOrder:    10,
		},
		{
			Name:         "日卡 100 美元",
			GroupName:    "日卡 100 美元",
			Description:  "20 元开通，24 小时总额度 100 美元",
			Price:        20,
			ValidityDays: 1,
			ValidityUnit: "days",
			Features:     "24 小时有效\n总额度 100 美元\n适合短时高强度使用",
			ProductName:  "日卡 100 美元额度",
			SortOrder:    20,
		},
		{
			Name:         "Lite 月卡",
			GroupName:    "Lite 月卡",
			Description:  "40 元月卡，每周 50 美元，月总额度 200 美元",
			Price:        40,
			ValidityDays: 1,
			ValidityUnit: "months",
			Features:     "每周限额 50 美元\n月总额度 200 美元\n适合轻度用户",
			ProductName:  "Lite 月卡",
			SortOrder:    30,
		},
		{
			Name:         "Standard 月卡",
			GroupName:    "Standard 月卡",
			Description:  "75 元月卡，每周 100 美元，月总额度 400 美元",
			Price:        75,
			ValidityDays: 1,
			ValidityUnit: "months",
			Features:     "每周限额 100 美元\n月总额度 400 美元\n适合主流用户",
			ProductName:  "Standard 月卡",
			SortOrder:    40,
		},
		{
			Name:         "Pro 月卡",
			GroupName:    "Pro 月卡",
			Description:  "140 元月卡，每周 250 美元，月总额度 1000 美元",
			Price:        140,
			ValidityDays: 1,
			ValidityUnit: "months",
			Features:     "每周限额 250 美元\n月总额度 1000 美元\n适合重度用户",
			ProductName:  "Pro 月卡",
			SortOrder:    50,
		},
		{
			Name:         "Ultra 月卡",
			GroupName:    "Ultra 月卡",
			Description:  "300 元月卡，每周 750 美元，月总额度 3000 美元",
			Price:        300,
			ValidityDays: 1,
			ValidityUnit: "months",
			Features:     "每周限额 750 美元\n月总额度 3000 美元\n适合超重度用户",
			ProductName:  "Ultra 月卡",
			SortOrder:    60,
		},
	}

	for _, seed := range seeds {
		groupID, ok := groupIDs[seed.GroupName]
		if !ok {
			return fmt.Errorf("missing group id for seeded plan %s", seed.Name)
		}

		existing, err := client.SubscriptionPlan.Query().
			Where(subscriptionplan.NameEQ(seed.Name)).
			Only(ctx)
		if err != nil && !dbent.IsNotFound(err) {
			return fmt.Errorf("query subscription plan %s: %w", seed.Name, err)
		}

		if existing == nil {
			_, createErr := client.SubscriptionPlan.Create().
				SetGroupID(groupID).
				SetName(seed.Name).
				SetDescription(seed.Description).
				SetPrice(seed.Price).
				SetValidityDays(seed.ValidityDays).
				SetValidityUnit(seed.ValidityUnit).
				SetFeatures(seed.Features).
				SetProductName(seed.ProductName).
				SetForSale(true).
				SetSortOrder(seed.SortOrder).
				Save(ctx)
			if createErr != nil {
				return fmt.Errorf("create subscription plan %s: %w", seed.Name, createErr)
			}
			continue
		}

		if _, updateErr := client.SubscriptionPlan.UpdateOneID(existing.ID).
			SetGroupID(groupID).
			SetDescription(seed.Description).
			SetPrice(seed.Price).
			SetValidityDays(seed.ValidityDays).
			SetValidityUnit(seed.ValidityUnit).
			SetFeatures(seed.Features).
			SetProductName(seed.ProductName).
			SetForSale(true).
			SetSortOrder(seed.SortOrder).
			Save(ctx); updateErr != nil {
			return fmt.Errorf("update subscription plan %s: %w", seed.Name, updateErr)
		}
	}

	return nil
}

func applyCPAPricingGroupLimitsCreate(builder *dbent.GroupCreate, seed cpaPricingGroupSeed) {
	builder.SetNillableDailyLimitUsd(seed.DailyLimitUSD)
	builder.SetNillableWeeklyLimitUsd(seed.WeeklyLimitUSD)
	builder.SetNillableMonthlyLimitUsd(seed.MonthlyLimitUSD)
}

func applyCPAPricingGroupLimitsUpdate(builder *dbent.GroupUpdateOne, seed cpaPricingGroupSeed) {
	if seed.DailyLimitUSD != nil {
		builder.SetDailyLimitUsd(*seed.DailyLimitUSD)
	} else {
		builder.ClearDailyLimitUsd()
	}
	if seed.WeeklyLimitUSD != nil {
		builder.SetWeeklyLimitUsd(*seed.WeeklyLimitUSD)
	} else {
		builder.ClearWeeklyLimitUsd()
	}
	if seed.MonthlyLimitUSD != nil {
		builder.SetMonthlyLimitUsd(*seed.MonthlyLimitUSD)
	} else {
		builder.ClearMonthlyLimitUsd()
	}
}

func float64Ptr(v float64) *float64 {
	return &v
}
