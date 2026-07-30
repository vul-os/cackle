// Turning an API error into a sentence a ticket buyer can act on.
//
// The visitor pages were passing `err.message` straight into <ErrorState>,
// so a buyer whose link had a typo in it read the words "ticket not found"
// — lowercase, no punctuation, no idea what to do next. That string is a
// wire-level status, not a message.
//
// This does not swallow information. A message the server went to the
// trouble of writing (a rate limit, a payment refusal, a network failure)
// is still shown, just sentence-cased. What gets replaced is the small set
// of bare status phrases that tell the reader nothing they can use.

const BARE_STATUS = /^(ticket|order|event|resource|page)?\s*not found\.?$/i;
const NOT_AUTHORISED = /^(unauthori[sz]ed|forbidden|access denied)\.?$/i;

/**
 * @param {string|null|undefined} raw the API's message
 * @param {string} fallback what to say when the API said nothing useful
 * @returns {string} a sentence
 */
export function humanError(raw, fallback) {
    const msg = (raw || '').trim();
    if (!msg) return fallback;
    if (BARE_STATUS.test(msg)) return fallback;
    if (NOT_AUTHORISED.test(msg)) return 'You need to be signed in as the person who bought this to see it.';
    // Sentence-case and terminate, so a lowercase fragment from the wire
    // still reads as prose next to the ones written by hand.
    const sentence = msg[0].toUpperCase() + msg.slice(1);
    return /[.!?]$/.test(sentence) ? sentence : `${sentence}.`;
}
