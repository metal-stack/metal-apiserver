package repository

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/stretchr/testify/require"
)

func Test_validate(t *testing.T) {

	var errs []error
	errs = validate(errs, false, "condition is false")

	require.Len(t, errs, 1)
	require.EqualError(t, errors.Join(errs...), "condition is false")

	var errs2 []error
	errs2 = validate(errs2, false, "condition 1 is false")
	errs2 = validate(errs2, false, "condition 2 is false")

	require.Len(t, errs2, 2)
	require.EqualError(t, errors.Join(errs2...), "condition 1 is false\ncondition 2 is false")
}

func Test_updateLabelsOnSlice(t *testing.T) {
	tests := []struct {
		name         string
		rq           *apiv2.UpdateLabels
		existingTags []string
		want         []string
	}{
		{
			name: "adding new labels",
			rq: &apiv2.UpdateLabels{
				Update: &apiv2.Labels{
					Labels: map[string]string{
						"a": "b",
						"c": "d",
					},
				},
			},
			existingTags: nil,
			want:         []string{"a=b", "c=d"},
		},
		{
			name: "adding new labels to existing ones",
			rq: &apiv2.UpdateLabels{
				Update: &apiv2.Labels{
					Labels: map[string]string{
						"a": "b",
						"c": "d",
					},
				},
			},
			existingTags: []string{"1=2", "foo"},
			want:         []string{"1=2", "a=b", "c=d", "foo"},
		},
		{
			name: "adding nothing maintains everything",
			rq: &apiv2.UpdateLabels{
				Update: &apiv2.Labels{},
			},
			existingTags: []string{"1=2", "foo="},
			want:         []string{"1=2", "foo="},
		},
		{
			name: "removing a label",
			rq: &apiv2.UpdateLabels{
				Remove: []string{"foo"},
			},
			existingTags: []string{"1=2", "foo"},
			want:         []string{"1=2"},
		},
		{
			name: "removing two labels",
			rq: &apiv2.UpdateLabels{
				Remove: []string{"foo", "1"},
			},
			existingTags: []string{"1=2", "foo"},
			want:         nil,
		},
		{
			name: "removing non-existent key is noop",
			rq: &apiv2.UpdateLabels{
				Remove: []string{"bar"},
			},
			existingTags: []string{"1=2", "foo="},
			want:         []string{"1=2", "foo="},
		},
		{
			name:         "existing tags without assignment are maintained",
			rq:           &apiv2.UpdateLabels{},
			existingTags: []string{"foo"},
			want:         []string{"foo"},
		},
		{
			name: "existing tags without assignment are maintained",
			rq: &apiv2.UpdateLabels{
				Update: &apiv2.Labels{
					Labels: map[string]string{
						"a=": "b",
					},
				},
			},
			existingTags: []string{"foo=="},
			want:         []string{"a==b", "foo=="},
		},
		{
			name: "transform as soon as user updates a pure label",
			rq: &apiv2.UpdateLabels{
				Update: &apiv2.Labels{
					Labels: map[string]string{
						"foo": "",
						"bar": "1",
					},
				},
			},
			existingTags: []string{"foo", "bar"},
			want:         []string{"bar=1", "foo="},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateLabelsOnSlice(tt.rq, tt.existingTags)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}

func Test_updateLabelsOnMap(t *testing.T) {
	tests := []struct {
		name         string
		rq           *apiv2.UpdateLabels
		existingTags map[string]string
		want         map[string]string
	}{
		{
			name:         "adding nothing",
			rq:           &apiv2.UpdateLabels{},
			existingTags: nil,
			want:         nil,
		},
		{
			name: "adding nothing if update is not nil",
			rq: &apiv2.UpdateLabels{
				Update: &apiv2.Labels{
					Labels: nil,
				},
			},
			existingTags: nil,
			want:         nil,
		},
		{
			name: "adding new labels",
			rq: &apiv2.UpdateLabels{
				Update: &apiv2.Labels{
					Labels: map[string]string{
						"a": "b",
						"c": "d",
					},
				},
			},
			existingTags: nil,
			want: map[string]string{
				"a": "b",
				"c": "d",
			},
		},
		{
			name: "adding new labels to existing ones",
			rq: &apiv2.UpdateLabels{
				Update: &apiv2.Labels{
					Labels: map[string]string{
						"a": "b",
						"c": "d",
					},
				},
			},
			existingTags: map[string]string{
				"1":   "2",
				"foo": "",
			},
			want: map[string]string{
				"a":   "b",
				"c":   "d",
				"1":   "2",
				"foo": "",
			},
		},
		{
			name: "removing a label",
			rq: &apiv2.UpdateLabels{
				Remove: []string{"foo"},
			},
			existingTags: map[string]string{
				"1":   "2",
				"foo": "",
			},
			want: map[string]string{
				"1": "2",
			},
		},
		{
			name: "removing two labels",
			rq: &apiv2.UpdateLabels{
				Remove: []string{"foo", "1"},
			},
			existingTags: map[string]string{
				"1":   "2",
				"foo": "",
			},
			want: map[string]string{},
		},
		{
			name: "removing non-existent key is noop",
			rq: &apiv2.UpdateLabels{
				Remove: []string{"bar"},
			},
			existingTags: map[string]string{
				"1":   "2",
				"foo": "",
			},
			want: map[string]string{
				"1":   "2",
				"foo": "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateLabelsOnMap(tt.rq, tt.existingTags)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}
