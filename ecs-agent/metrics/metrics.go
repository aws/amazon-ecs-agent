// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package metrics

// EntryFactory specifies the factory interface for creating new Metric Entry objects.
type EntryFactory interface {
	New(op string) Entry
	Flush()
}

// Entry specifies methods that need to be implemented by a metrics entry object.
type Entry interface {
	// WithFields allows the caller to set metric metadata. Once an entry object is
	// created (typically using a meaningful metrics name), its properties can be
	// manipulated using WithFields. Example:
	//
	// m := m.WithFields(map[string]interface{"timestamp": time.Now()})
	WithFields(f map[string]interface{}) Entry
	// WithCount allows the caller to set metric counts. This is useful for determining
	// if a particular operation has occurred or not. Example:
	//
	// item, ok := lookup(key)
	// if ok {
	//   lookupSuccessMetric = lookupSuccessMetric.WithCount(1)
	//   // Use item
	// } else {
	//   lookupSuccessMetric = lookupSuccessMetric.WithCount(0))
	// }
	WithCount(count int) Entry
	// WithGauge allows the caller to associate a specific value to the metric. This is useful
	// for reporting numerical values related to any operation, for instance the data transfer
	// rate for an image pull operation.
	WithGauge(value interface{}) Entry
	// Done makes a metric operation as complete. It records the end time of the operation
	// and returns a function pointer that can be used to flush the metrics to a
	// persistent store. Callers can optionally defer the function pointer. Example:
	//
	// defer lookupMetric.Done(nil) ()
	Done(err error) func()
}

// nopEntryFactory implements the EntryFactory interface with no-ops.
type nopEntryFactory struct{}

// NewNopEntryFactory creates a metric log entry factory that doesn't log any metrics.
func NewNopEntryFactory() EntryFactory {
	return &nopEntryFactory{}
}

func (*nopEntryFactory) New(op string) Entry {
	return newNopEntry(op)
}

func (f *nopEntryFactory) Flush() {}

type nopEntry struct{}

func newNopEntry(op string) Entry {
	return &nopEntry{}
}

func (e *nopEntry) WithFields(f map[string]interface{}) Entry { return e }
func (e *nopEntry) WithCount(count int) Entry                 { return e }
func (e *nopEntry) WithGauge(value interface{}) Entry         { return e }
func (e *nopEntry) Done(err error) func()                     { return func() {} }
