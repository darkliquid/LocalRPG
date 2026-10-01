// Narrative 2d6 Engine mechanics.
//
// The demo fixture uses this script so a screenshot session shows the
// mechanics strip populated. It is the same script the Worlds Studio offers as
// its reference template.

function resolve2d6(ctx) {
  var r = roll("2d6");
  var message = "";
  if (r.total >= 10) {
    message = "Strong Hit (Total: " + r.total + ") - complete triumph, no complications.";
  } else if (r.total >= 7) {
    message = "Weak Hit (Total: " + r.total + ") - success at a cost or complication.";
  } else {
    message = "Miss (Total: " + r.total + ") - the attempt falters; danger escalates.";
    injectGMDirection("The action failed. Introduce an immediate complication or escalate danger.");
  }
  return { success: r.total >= 7, message: message, roll: r };
}

onAction("do", resolve2d6);
onAction("say", resolve2d6);
onAction("story", resolve2d6);

onTurnEnd(function (ctx) {
  log("Turn " + ctx.turn + " completed in Narrative 2d6 Engine.");
});
