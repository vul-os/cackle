// lib/scan-store.js
//
// IndexedDB-backed local state for the offline gate scanner. Everything here
// must work with zero network access: the scan bundle is cached once while
// online (see `saveBundle`), and every scan after that is verified and
// recorded purely against this local database. Sync back to the server is a
// best-effort background step, never a precondition for admission.

import { openDB } from 'idb';
import { uuid } from './utils';

const DB_NAME = 'cackle-scanner';
const DB_VERSION = 1;
const STORE_BUNDLES = 'bundles';
const STORE_ADMISSIONS = 'admissions';

let dbPromise;

function getDB() {
    if (!dbPromise) {
        dbPromise = openDB(DB_NAME, DB_VERSION, {
            upgrade(db) {
                if (!db.objectStoreNames.contains(STORE_BUNDLES)) {
                    db.createObjectStore(STORE_BUNDLES, { keyPath: 'event_id' });
                }
                if (!db.objectStoreNames.contains(STORE_ADMISSIONS)) {
                    const store = db.createObjectStore(STORE_ADMISSIONS, { keyPath: 'id' });
                    store.createIndex('by_event', 'event_id');
                    store.createIndex('by_ticket', 'ticket_id');
                    store.createIndex('by_event_ticket', ['event_id', 'ticket_id']);
                    store.createIndex('by_synced', 'synced');
                }
            },
        });
    }
    return dbPromise;
}

/**
 * Cache a scan bundle fetched from GET /api/events/:id/scan-bundle. Shape
 * (per BUILD-SPEC): { event, issuer_keys[], ticket_index[], allocation, issued_at }.
 */
export async function saveBundle(eventId, bundle) {
    const db = await getDB();
    await db.put(STORE_BUNDLES, {
        event_id: eventId,
        ...bundle,
        cached_at: new Date().toISOString(),
    });
}

export async function getBundle(eventId) {
    const db = await getDB();
    return db.get(STORE_BUNDLES, eventId);
}

export async function listCachedBundles() {
    const db = await getDB();
    return db.getAll(STORE_BUNDLES);
}

export async function deleteBundle(eventId) {
    const db = await getDB();
    const tx = db.transaction([STORE_BUNDLES, STORE_ADMISSIONS], 'readwrite');
    await tx.objectStore(STORE_BUNDLES).delete(eventId);
    const admissionsStore = tx.objectStore(STORE_ADMISSIONS);
    const idx = admissionsStore.index('by_event');
    let cursor = await idx.openCursor(IDBKeyRange.only(eventId));
    while (cursor) {
        await cursor.delete();
        cursor = await cursor.continue();
    }
    await tx.done;
}

/** True if this ticket already has an 'admitted' row for this event. */
export async function wasAdmitted(eventId, ticketId) {
    const db = await getDB();
    const idx = db.transaction(STORE_ADMISSIONS).store.index('by_event_ticket');
    const rows = await idx.getAll(IDBKeyRange.only([eventId, ticketId]));
    return rows.some((r) => r.result === 'admitted');
}

/**
 * Append-only insert of a scan attempt. Never overwrites — duplicates get
 * their own row, mirroring internal/scan's server-side design so the local
 * mirror and the eventual server state agree once synced.
 */
export async function recordScan({ eventId, ticketId, deviceId, gateId, result, note = null, holderName = null, scannedAt }) {
    const db = await getDB();
    const record = {
        id: uuid(),
        event_id: eventId,
        ticket_id: ticketId ?? null,
        device_id: deviceId,
        gate_id: gateId,
        scanned_at: scannedAt || new Date().toISOString(),
        result, // 'admitted' | 'duplicate' | 'invalid' | 'wrong_event'
        note,
        holder_name: holderName,
        synced: 0,
    };
    await db.add(STORE_ADMISSIONS, record);
    return record;
}

export async function getAdmissionsForEvent(eventId) {
    const db = await getDB();
    return db.getAllFromIndex(STORE_ADMISSIONS, 'by_event', eventId);
}

export async function getTally(eventId) {
    const rows = await getAdmissionsForEvent(eventId);
    const tally = { admitted: 0, duplicate: 0, invalid: 0, wrong_event: 0, total: rows.length };
    for (const row of rows) {
        if (tally[row.result] !== undefined) tally[row.result]++;
    }
    return tally;
}

export async function getPendingSync(eventId) {
    const db = await getDB();
    const rows = await db.getAllFromIndex(STORE_ADMISSIONS, 'by_event', eventId);
    return rows.filter((r) => !r.synced);
}

export async function markSynced(ids) {
    if (!ids.length) return;
    const db = await getDB();
    const tx = db.transaction(STORE_ADMISSIONS, 'readwrite');
    for (const id of ids) {
        const row = await tx.store.get(id);
        if (row) {
            row.synced = 1;
            await tx.store.put(row);
        }
    }
    await tx.done;
}
