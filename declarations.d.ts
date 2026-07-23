/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * declarations.d.ts — TypeScript Module Declarations
 *
 * Declares ambient module types for non-TS asset imports (images, audio)
 * so TypeScript understands them as valid ES module default exports.
 */

declare module '*.jpg' {
  const value: string;
  export default value;
}

declare module '*.css';
