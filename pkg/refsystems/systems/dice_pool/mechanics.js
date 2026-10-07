// d10 pool: count the dice meeting the threshold and spend grit on a strong hit.
function resolvePool(ctx) {
  var size = getStat(ctx.player, "dice") || 5;
  var r = roll(size + "d10>=8");
  var successes = r.Successes || 0;

  if (successes >= 3) {
    var grit = getStat(ctx.player, "grit") || 0;
    setStat(ctx.player, "grit", Math.max(0, grit - 1));
    return { success: true, outcome: "strong", roll: r, message: successes + " successes: a strong hit; a point of grit is spent." };
  }
  if (successes >= 1) {
    return { success: true, outcome: "weak", roll: r, message: successes + " successes: a weak hit." };
  }
  return { success: false, outcome: "miss", roll: r, message: "No successes: the attempt misses." };
}

onAction("do", resolvePool);

onTurnEnd(function(ctx) {
  log("Turn " + ctx.turn + " completed in d10 Dice Pool.");
});
