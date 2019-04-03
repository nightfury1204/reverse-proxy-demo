package promquery

import (
	"fmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"strings"
)

// It will add label matchers to every metrics
func AddLabelMatchersToQuery(q string, labels []labels.Label) string {
	extraLbs := LabelsToString(labels)

	newQ := ""
	// all char are added to 'newQ' upto this position
	qPos := -1

	p := newParser(q)
	itm := p.next()
	for itm.typ != itemEOF {
		nextItm := p.next()
		if itm.typ == itemIdentifier || itm.typ == itemMetricIdentifier {
			if nextItm.typ != itemLeftParen { // to differential with function
				if nextItm.typ == itemLeftBrace { // http{} or http{method="get"}
					// copy all 'q' char from 'qPos+1' to 'nextItm.Pos'
					newQ = newQ + q[qPos+1:nextItm.pos+1]
					qPos = int(nextItm.pos)
					exists := false // already labels exists or not, e.g. http{}, http{method="get"}
					for nextItm.typ != itemRightBrace && nextItm.typ != itemEOF {
						nextItm = p.next()
						if nextItm.typ == itemString || nextItm.typ == itemIdentifier {
							exists = true
						}
					}

					newQ = newQ + extraLbs
					if exists {
						newQ = newQ + ","
					}
				} else { // http
					// copy all 'q' char from 'qPos+1' to 'itm.Pos + len(itm.val)'
					en := int(itm.pos)+len(itm.val)
					newQ = newQ + q[qPos+1:en] + fmt.Sprintf("{%s}",extraLbs)
					qPos = en-1
				}
			}
		}

		if itm.typ == itemLeftBrace { // handle case: {__name__=~"job:.*"}
			exists := false // '__name__' exists
			for nextItm.typ != itemRightBrace && nextItm.typ != itemEOF {
				if nextItm.typ == itemIdentifier && nextItm.val == model.MetricNameLabel {
					exists = true
				}
				nextItm = p.next()
			}

			if exists {
				// copy all 'q' char from 'qPos+1' to 'itm.Pos'
				newQ = newQ + q[qPos+1:itm.pos+1]
				qPos = int(itm.pos)
				newQ = newQ + extraLbs + ","
			}
		}

		if itm.typ > keywordsStart && itm.typ < keywordsEnd {
			if nextItm.typ == itemLeftParen {
				for nextItm.typ != itemRightParen && nextItm.typ != itemEOF {
					nextItm = p.next()
				}
			}
		}

		itm = nextItm
	}

	newQ = newQ + q[qPos+1:]
	return newQ
}

func LabelsToString(labels []labels.Label) string {
	lbs := []string{}
	for _, l := range labels {
		lbs = append(lbs, fmt.Sprintf(`%s="%s"`,l.Name, l.Value))
	}
	return strings.Join(lbs, ",")
}
