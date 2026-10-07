package oracle

import (
	"fmt"
	"math/rand"
	"strings"
)

var (
	confidentStrongOpeners = []string{
		"With effortless mastery and swift conviction, your move unfolds precisely as planned.",
		"Drawing upon seasoned instinct, you execute the maneuver with unshakeable confidence.",
		"Every motion is deliberate and assured; your decisive action commands the room.",
	}
	genericStrongOpeners = []string{
		"With practiced grace and sharp focus, your intent takes hold.",
		"The tides of fate answer your call; shadows part before your advance.",
		"Your action lands with resounding clarity across the chamber.",
		"Skill and fortune align as your effort carries through cleanly.",
	}
	weakOpeners = []string{
		"You gain ground, though not without feeling the cold sting of consequence.",
		"The maneuver succeeds, but the environment twists unexpectedly beneath your boots.",
		"A hard-won advantage, though eyes in the darkness take note of your position.",
		"Your effort achieves its mark, but at the cost of your balance and tempo.",
	}
	strainedMissOpeners = []string{
		"Straining against your limits, your footing falters and the gambit unravels.",
		"Desperation clouds your timing; the misstep leaves you vulnerable and reeling.",
		"Lacking the leverage you desperately need, your momentum collapses into disaster.",
	}
	genericMissOpeners = []string{
		"The darkness lashes out; your footing betrays you at the pivotal instant.",
		"A sudden jarring blow forces you back as the enemy anticipates your intent.",
		"The air turns freezing cold as the ancient wards shudder and resist.",
		"Fate turns against your endeavor, turning promise into sudden peril.",
	}
	neutralOpeners = []string{
		"You assess the unfolding scene and make your move with measured poise.",
		"The atmosphere hangs heavy as you commit to your course of action.",
		"Tension coils in the stillness before your effort ripples outward.",
	}

	strongStakesConsequences = []string{
		"You decisively keep the threat at bay—the risk that %s is averted.",
		"Decisive execution ensures that %s never comes to pass.",
		"You master the moment, firmly preventing the danger that %s.",
	}
	weakStakesConsequences = []string{
		"Yet complications emerge: the danger that %s remains an active threat.",
		"Even with the gain, you are left wrestling with the fact that %s.",
		"Progress is made, but the risk that %s looms larger than before.",
	}
	missStakesConsequences = []string{
		"Disaster strikes without mercy: %s.",
		"The worst comes to pass just as feared—%s.",
		"Unable to stem the tide, %s crashes down upon you.",
	}
	neutralStakesConsequences = []string{
		"The stakes hang unresolved: %s remains on the horizon.",
	}

	strongPlainConsequences = []string{
		"The path ahead clears, granting you a momentary tactical advantage.",
		"Your position strengthens as the immediate pressure subsides.",
		"The opposition recoils, granting you precious breathing room.",
	}
	weakPlainConsequences = []string{
		"A fleeting opportunity opens, but your margin for error has vanished.",
		"The immediate objective is met, but unintended noise draws unwanted attention.",
		"You hold the line for now, but the friction leaves you off-balance.",
	}
	missPlainConsequences = []string{
		"You are forced onto the defensive, scrambling to recover lost ground.",
		"The setback echoes immediately, narrowing your options for what comes next.",
		"Adversity takes hold, leaving you exposed to imminent danger.",
	}
	neutralPlainConsequences = []string{
		"The surrounding quiet breaks as the scene shifts around your intent.",
		"Every second counts as new variables enter the fray.",
	}

	castLineTemplates = []string{
		"Nearby, [[%s]] watches the outcome with bated breath.",
		"In the shadows close by, [[%s]] takes note of every motion.",
		"Across the clearing, [[%s]] reacts to the sudden shift in momentum.",
		"[[%s]] stands witness, eyes fixed on your next move.",
	}

	placeLineTemplates = []string{
		"The surrounding air of %s hangs heavy with the weight of the moment.",
		"Every stone and timber of %s seems to echo the clash.",
		"Silence settles briefly across %s as the dust begins to clear.",
		"The ancient atmosphere of %s braces for whatever follows.",
	}
)

func tierCategory(tier string) string {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "critical", "success", "full success", "full_success", "strong", "hit":
		return "strong"
	case "mixed", "partial", "complication", "weak", "glance":
		return "weak"
	case "miss", "fail", "failure":
		return "miss"
	default:
		return "neutral"
	}
}

func evalStat(p parts) (val int, hasStat bool) {
	if len(p.Stats) == 0 {
		return 0, false
	}
	actionLower := strings.ToLower(p.Action)
	for _, s := range p.Stats {
		if strings.Contains(actionLower, strings.ToLower(s.Name)) {
			return s.Value, true
		}
	}
	cat := tierCategory(p.Tier)
	if cat == "miss" {
		minVal := p.Stats[0].Value
		for _, s := range p.Stats[1:] {
			if s.Value < minVal {
				minVal = s.Value
			}
		}
		return minVal, true
	}
	maxVal := p.Stats[0].Value
	for _, s := range p.Stats[1:] {
		if s.Value > maxVal {
			maxVal = s.Value
		}
	}
	return maxVal, true
}

func opener(p parts, rng *rand.Rand) string {
	cat := tierCategory(p.Tier)
	val, hasStat := evalStat(p)

	switch cat {
	case "strong":
		if hasStat && val >= 2 {
			return confidentStrongOpeners[rng.Intn(len(confidentStrongOpeners))]
		}
		return genericStrongOpeners[rng.Intn(len(genericStrongOpeners))]
	case "weak":
		return weakOpeners[rng.Intn(len(weakOpeners))]
	case "miss":
		if hasStat && val <= 0 {
			return strainedMissOpeners[rng.Intn(len(strainedMissOpeners))]
		}
		return genericMissOpeners[rng.Intn(len(genericMissOpeners))]
	default:
		return neutralOpeners[rng.Intn(len(neutralOpeners))]
	}
}

func consequence(p parts, rng *rand.Rand) string {
	cat := tierCategory(p.Tier)
	stakes := strings.TrimSpace(p.Stakes)

	if stakes != "" {
		stakes = strings.TrimSuffix(stakes, ".")
		switch cat {
		case "strong":
			tmpl := strongStakesConsequences[rng.Intn(len(strongStakesConsequences))]
			return fmt.Sprintf(tmpl, stakes)
		case "weak":
			tmpl := weakStakesConsequences[rng.Intn(len(weakStakesConsequences))]
			return fmt.Sprintf(tmpl, stakes)
		case "miss":
			tmpl := missStakesConsequences[rng.Intn(len(missStakesConsequences))]
			return fmt.Sprintf(tmpl, stakes)
		default:
			tmpl := neutralStakesConsequences[rng.Intn(len(neutralStakesConsequences))]
			return fmt.Sprintf(tmpl, stakes)
		}
	}

	switch cat {
	case "strong":
		return strongPlainConsequences[rng.Intn(len(strongPlainConsequences))]
	case "weak":
		return weakPlainConsequences[rng.Intn(len(weakPlainConsequences))]
	case "miss":
		return missPlainConsequences[rng.Intn(len(missPlainConsequences))]
	default:
		return neutralPlainConsequences[rng.Intn(len(neutralPlainConsequences))]
	}
}

func castLine(p parts, rng *rand.Rand) string {
	if len(p.Entities) == 0 {
		return ""
	}
	ent := p.Entities[rng.Intn(len(p.Entities))]
	tmpl := castLineTemplates[rng.Intn(len(castLineTemplates))]
	return fmt.Sprintf(tmpl, ent)
}

func placeLine(p parts, rng *rand.Rand) string {
	loc := strings.TrimSpace(p.Location)
	if loc == "" {
		return ""
	}
	tmpl := placeLineTemplates[rng.Intn(len(placeLineTemplates))]
	return fmt.Sprintf(tmpl, loc)
}

