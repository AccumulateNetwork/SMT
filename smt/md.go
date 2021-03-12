package smt

import (
	"crypto/sha256"
)

// MS
// A Merkle Dag State is the state kept while building a Merkle Tree.  Except where a Merkle Tree has a clean
// power of two number of elements as leaf nodes, there will be multiple roots.  The combination of these
// roots provides a Directed Acyclic Graph (DAG) to all the leaves.
//
// Interestingly, the state of building such a Merkle Tree looks just like counting in binary.  And the
// higher order bits set will correspond to where the binary roots must be kept in a Merkle state.
type MS struct {
	HashFunction func(data []byte) Hash

	previous Hash    // Hash of the previous MS added to the Merkle Tree
	count    int64   // Count of hashes added to the Merkle tree
	Pending  []*Hash // Array of hashes that represent the left edge of the Merkle tree
	HashList []Hash  // List of Hashes in the order added to the chain
}

// Marshal
// Encodes the Merkle State so it can be embedded into the Merkle Tree
func (m *MS) Marshal() (MSBytes []byte) {
	// Add previous to MSBytes
	MSBytes = append(MSBytes, m.previous[:]...)
	// Add the count of all the hashes in the merkle tree to MSBytes
	MSBytes = append(MSBytes, Int64Bytes(m.count)...)
	// Add the Pending hashes to MSBytes; note we only have hashes where bits in count are set
	cnt := m.count             // Get the count of elements in the Merkle Tree
	for i := 0; cnt > 0; i++ { // For each bit in cnt,
		if cnt&1 > 0 { // if the bit is set in cnt, record the hash
			MSBytes = append(MSBytes, m.Pending[i][:]...)
		} // if the bit is not set, ignore (it is nil anyway)
		cnt = cnt >> 1 // Shift cnt so we can check the next bit
	}
	// Add the HashList to MSBytes.  First record the number of hashes added since last Merkle State
	MSBytes = append(MSBytes, Int64Bytes(int64(len(m.HashList)))...)
	// Now add all the Hashes in the HashList to MSBytes
	for _, v := range m.HashList {
		MSBytes = append(MSBytes, v[:]...)
	}

	return MSBytes
}

// UnMarshal
// Take the state of an MSMarshal instance defined by MSBytes, and set all the values
// in this instance of MSMarshal to the state defined by MSBytes.  It is assumed that the
// hash function has been set by the caller.
func (m *MS) UnMarshal(MSBytes []byte) {
	// Extract the previous hash from MSBytes
	copy(m.previous[:], MSBytes[:32])
	MSBytes = MSBytes[32:]
	// Extract the count of hashes in the Merkle Tree from MSBytes
	m.count, MSBytes = BytesInt64(MSBytes)
	// Extract the Pending roots array from MSBytes; not only where bits in count are set do we have a value in Pending
	cnt := m.count
	for i := 0; cnt > 0; i++ {
		// Add an nil element to Pending (for both if this bit in cnt is set or not
		m.Pending = append(m.Pending, nil)
		// If the cnt bit is set, we need to grab the hash out of MSBytes
		if cnt&1 > 0 {
			// Make this entry in Pending point to a hash
			m.Pending[i] = new(Hash)
			// Set the Hash to the value specified in MSBytes
			copy(m.Pending[i][:], MSBytes[:32])
			// Now skip MSBytes past the value we just copied to Pending
			MSBytes = MSBytes[32:]
		}
	}
	// Extract the length of the HashList
	var length int64
	length, MSBytes = BytesInt64(MSBytes)
	// For the length of the HashList, extract each Hash
	for i := int64(0); i < length; i++ {
		// First make room for one more Hash
		m.HashList = append(m.HashList, Hash{})
		// Copy over the Hash value
		copy(m.HashList[i][:], MSBytes[:32])
		// Move MSBytes over to the next hash value
		MSBytes = MSBytes[32:]
	}
}

func GetSha256() func(data []byte) Hash {
	return func(data []byte) Hash { return sha256.Sum256(data) }
}

func (m *MS) InitSha256() {
	m.HashFunction = GetSha256()
}

// AddToChain
// Add a Hash to the chain and incrementally build the MS
func (m *MS) AddToChain(hash Hash) {
	// We are going through through the MS list and combining hashes, so we have to record the hash first thing
	m.HashList = append(m.HashList, hash) // before it is combined with other hashes already added to MS[].

	// We make sure m.MS ends with a nil entry, because that cuts out most of the corner cases in adding hashes
	if len(m.Pending) == 0 || m.Pending[len(m.Pending)-1] != nil { // If first entry, or the last entry isn't nil
		m.Pending = append(m.Pending, nil) // then we need to add a nil to the end of m.MS
	}

	// Okay, now we go through m.Pending and look for the first nil entry in Pending and add our hash there. Along the
	// way, we take every non-vil entry and combine it with the hash we are adding. Note we ALWAYS have a nil at the
	// end of m.MS so we don't have a end case to deal with.
	for i, v := range m.Pending {

		// Look and see if the current spot in MS is open.
		if v == nil { // If it is open, put our hash here and continue.
			m.Pending[i] = &hash // put a pointer to a copy of hash into m.MS
			return               // If we have added the hash to m.MS then we are done.
		}

		// If the current spot is NOT open, we need to combine the hash we have with the hash on the "left", i.e.
		// the hash already in m.Pending
		hash = v.Combine(m.HashFunction, hash) // Combine v (left) and hash (right) to get a new combined hash to use forward
		m.Pending[i] = nil                     // Now that we have combined v and hash, this spot is now empty, so clear it.
	}
}

// GetMDRoot
// Close off the Merkle Directed Acyclic Graph (Merkle DAG or MS)
// We take any trailing hashes in MS, hash them up and combine to create the Merkle Dag Root.
// Getting the closing ListMDRoot is non-destructive, which is useful for some use cases.
func (m *MS) GetMDRoot() (MDRoot *Hash) {
	// We go through m.MS and combine any left over hashes in m.MS with each other and the MR.
	// If this is a power of two, that's okay because we will pick up the MR (a balanced MS) and
	// return that, the correct behavior
	for _, v := range m.Pending {
		if MDRoot == nil { // We will pick up the first hash in m.MS no matter what.
			MDRoot = v // If we assign a nil over a nil, no harm no foul.  Fewer cases to test this way.
		} else if v != nil { // If MDRoot isn't nil and v isn't nil, we combine them.
			combine := v.Combine(m.HashFunction, *MDRoot) // v is on the left, MDRoot candidate is on the right, for a new MDRoot
			MDRoot = &combine
		}
	}
	// We drop out with a MDRoot unless m.MS is zero length, in which case we return a nil (correct)
	// If m.MS has the entries for a power of two, then only one hash (the last) is in m.MS, which we return (correct)
	// If m.MS has a railing nil, we return the trailing entries combined with the last entry in m.MS (correct)
	return MDRoot
}

// PrintMR
// For debugging purposes, it is nice to get a string that shows the nil and non nil entries in c.MS
// Note that the "low order" entries are first in the string, so the binary is going from low order on the left to
// high order going right in the string rather than how binary is normally represented.
func (m *MS) PrintMR() (mr string) {
	for _, v := range m.Pending {
		if v != nil {
			mr += "O"
			continue
		}
		mr += "_"
	}
	return mr
}