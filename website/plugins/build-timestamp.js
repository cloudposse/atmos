'use strict';

const SOURCE_DATE_EPOCH = 'SOURCE_DATE_EPOCH';

/**
 * Return the reproducible build timestamp when SOURCE_DATE_EPOCH is set.
 * Fall back to wall-clock time for local builds that do not request
 * reproducibility.
 *
 * @returns {Date}
 */
function getBuildDate() {
  const epoch = process.env[SOURCE_DATE_EPOCH];
  if (epoch === undefined || epoch === '') {
    return new Date();
  }

  if (!/^\d+$/.test(epoch)) {
    throw new Error(`${SOURCE_DATE_EPOCH} must be a non-negative integer number of seconds`);
  }

  const date = new Date(Number(epoch) * 1000);
  if (Number.isNaN(date.getTime())) {
    throw new Error(`${SOURCE_DATE_EPOCH} is outside the supported date range`);
  }

  return date;
}

function getBuildTimestamp() {
  return getBuildDate().toISOString();
}

module.exports = {
  getBuildDate,
  getBuildTimestamp,
};
