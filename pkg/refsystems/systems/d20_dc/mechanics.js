// d20 + DC: roll a d20, add the governing stat, and compare the total to a class.
function resolveD20(ctx) {
  var modifier = getStat(ctx.player, "strength") || 0;
  var r = roll("1d20");
  var total = r.Total + modifier;
  var dc = 15;
  if (total >= dc) {
    return { success: true, outcome: "success", roll: r, message: "Total " + total + " meets DC " + dc + "." };
  }
  return { success: false, outcome: "fail", roll: r, message: "Total " + total + " misses DC " + dc + "." };
}

onAction("do", resolveD20);

onTurnEnd(function(ctx) {
  log("Turn " + ctx.turn + " completed in d20 + DC.");
});
