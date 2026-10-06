// ==========================================
// Narrative 2d6 Engine - Mechanics Script
// ==========================================

function resolve2d6(ctx) {
  var r = roll("2d6");
  var message = "";
  if (r.Total >= 10) {
    message = "Strong Hit (Total: " + r.Total + ") - complete triumph, no complications.";
  } else if (r.Total >= 7) {
    message = "Weak Hit (Total: " + r.Total + ") - success at a cost or complication.";
  } else {
    message = "Miss (Total: " + r.Total + ") - the attempt falters; danger escalates.";
    injectGMDirection("The action failed. Introduce an immediate complication or escalate danger.");
  }
  return { success: r.Total >= 7, outcome: r.Total >= 10 ? "strong" : (r.Total >= 7 ? "weak" : "miss"), message: message, roll: r };
}

onAction("do", resolve2d6);
onAction("say", resolve2d6);
onAction("story", resolve2d6);

onTurnEnd(function(ctx) {
  log("Turn " + ctx.turn + " completed in Narrative 2d6 Engine.");
});
