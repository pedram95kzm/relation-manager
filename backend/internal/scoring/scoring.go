package scoring

import (
	"math"

	"relations/internal/questionnaire"
)

// Required uses exact two-thirds; a rounded decimal would incorrectly require
// all three responses in a three-item dimension.
func Required(total int) int { return max(2, (2*total+2)/3) }

func Compute(q questionnaire.Definition, answers questionnaire.Answers, ctx questionnaire.Context) (Result, error) {
	if err := questionnaire.Validate(q, answers, ctx, false); err != nil {
		return Result{}, err
	}
	result := Result{Module: q.Module, QuestionnaireVersion: q.Version, ScoringVersion: q.ScoringVersion,
		Status: "insufficient_data", Respondents: make(map[string]RespondentScores)}
	result.Respondents["self"] = respondent(q, answers["self"])
	if q.Module == "compatibility" {
		result.Respondents["partner"] = respondent(q, answers["partner"])
		comparison := similarity(q, result.Respondents["self"], result.Respondents["partner"], ctx)
		result.Compatibility = &comparison
		if comparison.Status == "available" {
			result.Status = "available"
		}
	} else if q.Module == "conflict" {
		if questionnaire.ValidateContext(q.Module, ctx, true) == nil {
			result.Status = "available"
		}
	} else if sufficientCount(result.Respondents["self"]) >= 3 {
		result.Status = "available"
	}
	return result, nil
}

func respondent(q questionnaire.Definition, values map[string]*int) RespondentScores {
	output := RespondentScores{Dimensions: make([]DimensionScore, 0, len(q.Dimensions)), Total: len(q.Questions)}
	for _, dimension := range q.Dimensions {
		score := DimensionScore{ID: dimension.ID, Label: dimension.Label, Status: "insufficient_data"}
		sum := 0
		for _, question := range q.Questions {
			if question.Dimension != dimension.ID {
				continue
			}
			score.Total++
			value, present := values[question.ID]
			if !present {
				score.Omitted++
			} else if value == nil {
				score.Unknown++
			} else {
				score.Answered++
				keyed := *value
				if question.Reverse {
					keyed = 6 - keyed
				}
				sum += keyed
			}
		}
		score.Completeness = float64(score.Answered) / float64(score.Total)
		if score.Answered >= Required(score.Total) {
			value := 25 * (float64(sum)/float64(score.Answered) - 1)
			band := "middle"
			if value < 40 {
				band = "lower"
			} else if value > 60 {
				band = "higher"
			}
			score.Score, score.Band, score.Status = &value, &band, "available"
		}
		output.Answered += score.Answered
		output.Unknown += score.Unknown
		output.Omitted += score.Omitted
		output.Dimensions = append(output.Dimensions, score)
	}
	output.NumericCompleteness = float64(output.Answered) / float64(output.Total)
	return output
}

func sufficientCount(scores RespondentScores) int {
	count := 0
	for _, dimension := range scores.Dimensions {
		if dimension.Score != nil {
			count++
		}
	}
	return count
}

func similarity(q questionnaire.Definition, self, partner RespondentScores, ctx questionnaire.Context) Similarity {
	output := Similarity{Status: "insufficient_data", Components: []SimilarityComponent{}, IncludedDimensions: []string{},
		OmittedDimensions: []string{}, Formula: "mean(100 - abs(self_dimension_score - partner_dimension_score)) across sufficient common dimensions; minimum 3"}
	allowed := ctx.PartnerResponseOrigin == "independent" && ctx.PartnerConsent
	sum := 0.0
	for i, dimension := range q.Dimensions {
		left, right := self.Dimensions[i], partner.Dimensions[i]
		if !allowed || left.Score == nil || right.Score == nil {
			output.OmittedDimensions = append(output.OmittedDimensions, dimension.ID)
			continue
		}
		value := 100 - math.Abs(*left.Score-*right.Score)
		sum += value
		output.Components = append(output.Components, SimilarityComponent{DimensionID: dimension.ID, Similarity: value})
		output.IncludedDimensions = append(output.IncludedDimensions, dimension.ID)
	}
	output.Coverage = float64(len(output.Components)) / float64(len(q.Dimensions))
	if !allowed {
		output.Status = "requires_independent_responses"
	} else if len(output.Components) >= 3 {
		value := sum / float64(len(output.Components))
		output.Index, output.Status = &value, "available"
	}
	return output
}
