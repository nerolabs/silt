package credit

// Test helpers for the G-R212-7 numéraire (2026-09-06): serves are sized in mint
// UNITS so the two-floor lane arithmetic is exact and the pre-numéraire conservation
// gates keep their discriminating power. One unit = SkimDen·Dλ bytes on the object
// path mints exactly SkimDen−SkimNum credits net and SkimNum skimmed; on the plain
// path Dλ bytes mint one credit.
const mintUnit = int64(SkimDen * ServeMintBytesPerCredit) // 3,145,728 bytes

// objNet / objSkim mirror the certified two-floor split for a lane that has served
// exactly `bytes` in total. They are the ORACLE for conservation tests only; the
// formula itself is pinned by literal numbers in TestGLambda4ChunkingInvariance and TestGLambda7SkimAccumulatesAcrossServes.
func objNet(bytes int64) int64 {
	return bytes * (SkimDen - SkimNum) / (SkimDen * ServeMintBytesPerCredit)
}
func objSkim(bytes int64) int64 { return bytes * SkimNum / (SkimDen * ServeMintBytesPerCredit) }
