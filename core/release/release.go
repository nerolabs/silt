// Package release names the build a node is running, as one monotonic number.
//
// A version-floor advisory (core/advisory) names the lowest release the network asks its
// validators to run; a node compares its own Number against it, and advertises Number to its
// peers so a patched validator can tell an old one from a current one. The number only ever
// increases, one step per release, so "below the floor" is a plain comparison.
package release

// Number is this build's release. It is raised by one in the commit that cuts a release.
const Number uint32 = 1
