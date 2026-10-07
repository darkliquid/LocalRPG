import { TurnSegment } from '../types';

// clipKeyFromURL reads the content key out of a clip URL. The key is the audio's
// identity, so a played-set comparison needs nothing but the URL itself.
export const clipKeyFromURL = (url: string): string => {
  const path = url.split('?')[0];
  const parts = path.split('/');
  return parts[parts.length - 1] ?? '';
};

// segmentClipURLs is the ordered list of clip URLs a segment plays.
export const segmentClipURLs = (segment: TurnSegment | undefined): string[] =>
  (segment?.audio_urls ?? []).filter((url) => !!url);

// AnySegmentHasAudio reports whether a turn is worth narrating at all.
export const anySegmentHasAudio = (segments: TurnSegment[] | undefined): boolean =>
  (segments ?? []).some((segment) => segmentClipURLs(segment).length > 0);

// segmentIsGroupLeader reports whether a segment is the first of its clip group,
// which is where the shared audio control renders. A segment with no group is its
// own leader, so an ungrouped turn keeps one control per segment.
export const segmentIsGroupLeader = (segments: TurnSegment[] | undefined, index: number): boolean => {
  const group = segments?.[index]?.clip_group;
  if (!group) return true;
  for (let i = 0; i < index; i += 1) {
    if (segments?.[i]?.clip_group === group) return false;
  }
  return true;
};

// groupLeaderIndex returns the first segment of the clip group the index belongs
// to, or the index itself when it is ungrouped. A group shares one clip, so this
// is the index whose audio a viewer hears while stepping through the group.
export const groupLeaderIndex = (segments: TurnSegment[] | undefined, index: number): number => {
  const group = segments?.[index]?.clip_group;
  if (!group) return index;
  for (let i = 0; i < index; i += 1) {
    if (segments?.[i]?.clip_group === group) return i;
  }
  return index;
};

// groupLastIndex returns the last segment of the clip group the index belongs to,
// or the index itself when it is ungrouped. Groups are contiguous runs, so a
// forward scan ends at the first segment of a different group.
export const groupLastIndex = (segments: TurnSegment[] | undefined, index: number): number => {
  const group = segments?.[index]?.clip_group;
  if (!group) return index;
  let last = index;
  for (let i = index + 1; i < (segments?.length ?? 0); i += 1) {
    if (segments?.[i]?.clip_group === group) last = i;
    else if (segments?.[i]?.clip_group) break;
  }
  return last;
};

