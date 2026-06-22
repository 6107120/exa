package exa

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestExa_KoreanDependency_AutoResolution(t *testing.T) {
	// [검증] evidence 맵 우회나 하위 호환용 찌꺼기 없이, 완벽하게 선언적으로 작성된 순수 한글 계산식!
	req := Request{
		Inputs: map[string]any{
			"기본급": "5000000",
		},
		Policy: []Calculation{
			{
				ID:         "실수령액",
				Expression: "기본급 - 세금_5프로 - 건강보험료",
			},
			{
				ID:         "세금_5프로",
				Expression: "기본급 * 0.05",
			},
			{
				ID:         "건강보험료",
				Expression: "기본급 * 0.03",
			},
		},
	}

	engine := NewEngine()
	res, err := engine.Compute(context.Background(), req)
	assert.NoError(t, err)

	// 세금_5프로 = 5000000 * 0.05 = 250000
	assert.True(t, res["세금_5프로"].Equal(decimal.NewFromInt(250000)))
	// 건강보험료 = 5000000 * 0.03 = 150000
	assert.True(t, res["건강보험료"].Equal(decimal.NewFromInt(150000)))
	// 실수령액 = 5000000 - 250000 - 150000 = 4600000
	assert.True(t, res["실수령액"].Equal(decimal.NewFromInt(4600000)))
}

func TestExa_KoreanDependency_CircularDetection(t *testing.T) {
	req := Request{
		Inputs: map[string]any{},
		Policy: []Calculation{
			{
				ID:         "갑",
				Expression: "을 + 10",
			},
			{
				ID:         "을",
				Expression: "갑 * 2",
			},
		},
	}

	engine := NewEngine()
	_, err := engine.Compute(context.Background(), req)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCircularDependency)
}

func TestExa_GlobalUnicode_Multilingual(t *testing.T) {
	// [검증] 한글뿐만 아니라 한자(중국어/일어), 가타카나, 히라가나 등 전세계 모든 유니코드 문자가
	// 완벽하게 컴파일 및 의존성 정렬을 거쳐 올바르게 계산되는지 검증!
	req := Request{
		Inputs: map[string]any{
			"基本給": "400000", // 한자 (Basic Salary)
		},
		Policy: []Calculation{
			{
				ID:         "총급여", // 히라가나 (Net Salary)
				Expression: "基本給 - 税金_5パーセント",
			},
			{
				ID:         "税金_5パーセント", // 가타카나 (Tax 5%)
				Expression: "基本給 * 0.05",
			},
		},
	}

	engine := NewEngine()
	res, err := engine.Compute(context.Background(), req)
	assert.NoError(t, err)

	// 税金_5パーセント = 400000 * 0.05 = 20000
	assert.True(t, res["税金_5パーセント"].Equal(decimal.NewFromInt(20000)))
	// 총급여 = 400000 - 20000 = 380000
	assert.True(t, res["총급여"].Equal(decimal.NewFromInt(380000)))
}

func TestExa_Unicode_Normalization_And_Sanitization(t *testing.T) {
	// 1. NFD (Decomposed Jamo) Representation of "기본급" (macOS Style)
	// '기' decomposes to \u1100 (ㄱ) + \u1175 (ㅣ)
	// '본' decomposes to \u1107 (ㅂ) + \u1169 (ㅗ) + \u11ab (ㄴ)
	// '급' decomposes to \u1100 (ㄱ) + \u1173 (ㅡ) + \u11b8 (ㅂ)
	nfdKey := "\u1100\u1175\u1107\u1169\u11ab\u1100\u1173\u11b8" // "기본급" under NFD
	
	// 2. Variable name with invisible zero-width spaces and NBSPs: "세금_5프로" with trailing \u200b and NBSP \u00a0
	dirtyTaxKey := "세금_5프로\u200b\u00a0"

	req := Request{
		Inputs: map[string]any{
			nfdKey: "5000000", // Will be normalized to NFC "기본급"
		},
		Policy: []Calculation{
			{
				ID:         dirtyTaxKey, // Will be cleaned to "세금_5프로"
				Expression: "기본급 * 0.05", // Uses standard NFC "기본급"
			},
			{
				ID:         "실수령액",
				Expression: "기본급 - 세금_5프로", // Uses cleaned "세금_5프로"
			},
		},
	}

	engine := NewEngine()
	res, err := engine.Compute(context.Background(), req)
	assert.NoError(t, err)

	// Keys in res should be returned in beautiful, normalized NFC
	assert.True(t, res["세금_5프로"].Equal(decimal.NewFromInt(250000)))
	assert.True(t, res["실수령액"].Equal(decimal.NewFromInt(4750000)))
}

func TestExa_Error_Deobfuscation(t *testing.T) {
	// Verify that error messages do not leak internal "_u_" representations.
	engine := NewEngine()

	t.Run("Duplicate Identifier Error", func(t *testing.T) {
		req := Request{
			Inputs: map[string]any{
				"기본급": "5000000",
			},
			Policy: []Calculation{
				{
					ID:         "기본급", // Duplicate ID
					Expression: "100",
				},
			},
		}
		_, err := engine.Compute(context.Background(), req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "기본급")
		assert.NotContains(t, err.Error(), "_u_")
	})

	t.Run("Circular Dependency Error", func(t *testing.T) {
		req := Request{
			Inputs: map[string]any{},
			Policy: []Calculation{
				{
					ID:         "환급금",
					Expression: "세금 + 10",
				},
				{
					ID:         "세금",
					Expression: "환급금 * 0.1",
				},
			},
		}
		_, err := engine.Compute(context.Background(), req)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCircularDependency)
		assert.Contains(t, err.Error(), "환급금")
		assert.NotContains(t, err.Error(), "_u_")
	})

	t.Run("Compilation / Type Check Error", func(t *testing.T) {
		req := Request{
			Inputs: map[string]any{
				"수당": "not_a_number", // String instead of decimal
			},
			Policy: []Calculation{
				{
					ID:         "보너스",
					Expression: "수당 * 2", // Causes dynamic eval/check error depending on CEL environment
				},
			},
		}
		_, err := engine.Compute(context.Background(), req)
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "_u_")
	})
}
