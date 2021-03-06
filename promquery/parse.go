// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promquery

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/value"

	"github.com/prometheus/prometheus/util/strutil"
)

type parser struct {
	lex       *lexer
	token     [3]item
	peekCount int
}

// ParseErr wraps a parsing error with line and position context.
// If the parsing input was a single line, line will be 0 and omitted
// from the error string.
type ParseErr struct {
	Line, Pos int
	Err       error
}

func (e *ParseErr) Error() string {
	if e.Line == 0 {
		return fmt.Sprintf("parse error at char %d: %s", e.Pos, e.Err)
	}
	return fmt.Sprintf("parse error at line %d, char %d: %s", e.Line, e.Pos, e.Err)
}

// ParseMetric parses the input into a metric
func ParseMetric(input string) (m labels.Labels, err error) {
	p := newParser(input)
	defer p.recover(&err)

	m = p.metric()
	if p.peek().typ != itemEOF {
		p.errorf("could not parse remaining input %.15q...", p.lex.input[p.lex.lastPos:])
	}
	return m, nil
}

// newParser returns a new parser.
func newParser(input string) *parser {
	p := &parser{
		lex: lex(input),
	}
	return p
}

// sequenceValue is an omittable value in a sequence of time series values.
type sequenceValue struct {
	value   float64
	omitted bool
}

func (v sequenceValue) String() string {
	if v.omitted {
		return "_"
	}
	return fmt.Sprintf("%f", v.value)
}

// parseSeriesDesc parses the description of a time series.
func parseSeriesDesc(input string) (labels.Labels, []sequenceValue, error) {
	p := newParser(input)
	p.lex.seriesDesc = true

	return p.parseSeriesDesc()
}

// parseSeriesDesc parses a description of a time series into its metric and value sequence.
func (p *parser) parseSeriesDesc() (m labels.Labels, vals []sequenceValue, err error) {
	defer p.recover(&err)

	m = p.metric()

	const ctx = "series values"
	for {
		for p.peek().typ == itemSpace {
			p.next()
		}
		if p.peek().typ == itemEOF {
			break
		}

		// Extract blanks.
		if p.peek().typ == itemBlank {
			p.next()
			times := uint64(1)
			if p.peek().typ == itemTimes {
				p.next()
				times, err = strconv.ParseUint(p.expect(itemNumber, ctx).val, 10, 64)
				if err != nil {
					p.errorf("invalid repetition in %s: %s", ctx, err)
				}
			}
			for i := uint64(0); i < times; i++ {
				vals = append(vals, sequenceValue{omitted: true})
			}
			// This is to ensure that there is a space between this and the next number.
			// This is especially required if the next number is negative.
			if t := p.expectOneOf(itemSpace, itemEOF, ctx).typ; t == itemEOF {
				break
			}
			continue
		}

		// Extract values.
		sign := 1.0
		if t := p.peek().typ; t == itemSUB || t == itemADD {
			if p.next().typ == itemSUB {
				sign = -1
			}
		}
		var k float64
		if t := p.peek().typ; t == itemNumber {
			k = sign * p.number(p.expect(itemNumber, ctx).val)
		} else if t == itemIdentifier && p.peek().val == "stale" {
			p.next()
			k = math.Float64frombits(value.StaleNaN)
		} else {
			p.errorf("expected number or 'stale' in %s but got %s (value: %s)", ctx, t.desc(), p.peek())
		}
		vals = append(vals, sequenceValue{
			value: k,
		})

		// If there are no offset repetitions specified, proceed with the next value.
		if t := p.peek(); t.typ == itemSpace {
			// This ensures there is a space between every value.
			continue
		} else if t.typ == itemEOF {
			break
		} else if t.typ != itemADD && t.typ != itemSUB {
			p.errorf("expected next value or relative expansion in %s but got %s (value: %s)", ctx, t.desc(), p.peek())
		}

		// Expand the repeated offsets into values.
		sign = 1.0
		if p.next().typ == itemSUB {
			sign = -1.0
		}
		offset := sign * p.number(p.expect(itemNumber, ctx).val)
		p.expect(itemTimes, ctx)

		times, err := strconv.ParseUint(p.expect(itemNumber, ctx).val, 10, 64)
		if err != nil {
			p.errorf("invalid repetition in %s: %s", ctx, err)
		}

		for i := uint64(0); i < times; i++ {
			k += offset
			vals = append(vals, sequenceValue{
				value: k,
			})
		}
		// This is to ensure that there is a space between this expanding notation
		// and the next number. This is especially required if the next number
		// is negative.
		if t := p.expectOneOf(itemSpace, itemEOF, ctx).typ; t == itemEOF {
			break
		}
	}
	return m, vals, nil
}

// next returns the next token.
func (p *parser) next() item {
	if p.peekCount > 0 {
		p.peekCount--
	} else {
		t := p.lex.nextItem()
		// Skip comments.
		for t.typ == itemComment {
			t = p.lex.nextItem()
		}
		p.token[0] = t
	}
	if p.token[p.peekCount].typ == itemError {
		p.errorf("%s", p.token[p.peekCount].val)
	}
	return p.token[p.peekCount]
}

// peek returns but does not consume the next token.
func (p *parser) peek() item {
	if p.peekCount > 0 {
		return p.token[p.peekCount-1]
	}
	p.peekCount = 1

	t := p.lex.nextItem()
	// Skip comments.
	for t.typ == itemComment {
		t = p.lex.nextItem()
	}
	p.token[0] = t
	return p.token[0]
}

// backup backs the input stream up one token.
func (p *parser) backup() {
	p.peekCount++
}

// errorf formats the error and terminates processing.
func (p *parser) errorf(format string, args ...interface{}) {
	p.error(fmt.Errorf(format, args...))
}

// error terminates processing.
func (p *parser) error(err error) {
	perr := &ParseErr{
		Line: p.lex.lineNumber(),
		Pos:  p.lex.linePosition(),
		Err:  err,
	}
	if strings.Count(strings.TrimSpace(p.lex.input), "\n") == 0 {
		perr.Line = 0
	}
	panic(perr)
}

// expect consumes the next token and guarantees it has the required type.
func (p *parser) expect(exp ItemType, context string) item {
	token := p.next()
	if token.typ != exp {
		p.errorf("unexpected %s in %s, expected %s", token.desc(), context, exp.desc())
	}
	return token
}

// expectOneOf consumes the next token and guarantees it has one of the required types.
func (p *parser) expectOneOf(exp1, exp2 ItemType, context string) item {
	token := p.next()
	if token.typ != exp1 && token.typ != exp2 {
		p.errorf("unexpected %s in %s, expected %s or %s", token.desc(), context, exp1.desc(), exp2.desc())
	}
	return token
}

var errUnexpected = fmt.Errorf("unexpected error")

// recover is the handler that turns panics into returns from the top level of Parse.
func (p *parser) recover(errp *error) {
	e := recover()
	if e != nil {
		if _, ok := e.(runtime.Error); ok {
			// Print the stack trace but do not inhibit the running application.
			buf := make([]byte, 64<<10)
			buf = buf[:runtime.Stack(buf, false)]

			fmt.Fprintf(os.Stderr, "parser panic: %v\n%s", e, buf)
			*errp = errUnexpected
		} else {
			*errp = e.(error)
		}
	}
	p.lex.close()
}

// number parses a number.
func (p *parser) number(val string) float64 {
	n, err := strconv.ParseInt(val, 0, 64)
	f := float64(n)
	if err != nil {
		f, err = strconv.ParseFloat(val, 64)
	}
	if err != nil {
		p.errorf("error parsing number: %s", err)
	}
	return f
}

// labels parses a list of labelnames.
//
//		'(' <label_name>, ... ')'
//
func (p *parser) labels() []string {
	const ctx = "grouping opts"

	p.expect(itemLeftParen, ctx)

	labels := []string{}
	if p.peek().typ != itemRightParen {
		for {
			id := p.next()
			if !isLabel(id.val) {
				p.errorf("unexpected %s in %s, expected label", id.desc(), ctx)
			}
			labels = append(labels, id.val)

			if p.peek().typ != itemComma {
				break
			}
			p.next()
		}
	}
	p.expect(itemRightParen, ctx)

	return labels
}

// labelSet parses a set of label matchers
//
//		'{' [ <labelname> '=' <match_string>, ... ] '}'
//
func (p *parser) labelSet() labels.Labels {
	set := []labels.Label{}
	for _, lm := range p.labelMatchers(itemEQL) {
		set = append(set, labels.Label{Name: lm.Name, Value: lm.Value})
	}
	return labels.New(set...)
}

// labelMatchers parses a set of label matchers.
//
//		'{' [ <labelname> <match_op> <match_string>, ... ] '}'
//
func (p *parser) labelMatchers(operators ...ItemType) []*labels.Matcher {
	const ctx = "label matching"

	matchers := []*labels.Matcher{}

	p.expect(itemLeftBrace, ctx)

	// Check if no matchers are provided.
	if p.peek().typ == itemRightBrace {
		p.next()
		return matchers
	}

	for {
		label := p.expect(itemIdentifier, ctx)

		op := p.next().typ
		if !op.isOperator() {
			p.errorf("expected label matching operator but got %s", op)
		}
		var validOp = false
		for _, allowedOp := range operators {
			if op == allowedOp {
				validOp = true
			}
		}
		if !validOp {
			p.errorf("operator must be one of %q, is %q", operators, op)
		}

		val := p.unquoteString(p.expect(itemString, ctx).val)

		// Map the item to the respective match type.
		var matchType labels.MatchType
		switch op {
		case itemEQL:
			matchType = labels.MatchEqual
		case itemNEQ:
			matchType = labels.MatchNotEqual
		case itemEQLRegex:
			matchType = labels.MatchRegexp
		case itemNEQRegex:
			matchType = labels.MatchNotRegexp
		default:
			p.errorf("item %q is not a metric match type", op)
		}

		m, err := labels.NewMatcher(matchType, label.val, val)
		if err != nil {
			p.error(err)
		}

		matchers = append(matchers, m)

		if p.peek().typ == itemIdentifier {
			p.errorf("missing comma before next identifier %q", p.peek().val)
		}

		// Terminate list if last matcher.
		if p.peek().typ != itemComma {
			break
		}
		p.next()

		// Allow comma after each item in a multi-line listing.
		if p.peek().typ == itemRightBrace {
			break
		}
	}

	p.expect(itemRightBrace, ctx)

	return matchers
}

// metric parses a metric.
//
//		<label_set>
//		<metric_identifier> [<label_set>]
//
func (p *parser) metric() labels.Labels {
	name := ""
	var m labels.Labels

	t := p.peek().typ
	if t == itemIdentifier || t == itemMetricIdentifier {
		name = p.next().val
		t = p.peek().typ
	}
	if t != itemLeftBrace && name == "" {
		p.errorf("missing metric name or metric selector")
	}
	if t == itemLeftBrace {
		m = p.labelSet()
	}
	if name != "" {
		m = append(m, labels.Label{Name: labels.MetricName, Value: name})
		sort.Sort(m)
	}
	return m
}

// offset parses an offset modifier.
//
//		offset <duration>
//
func (p *parser) offset() time.Duration {
	const ctx = "offset"

	p.next()
	offi := p.expect(itemDuration, ctx)

	offset, err := parseDuration(offi.val)
	if err != nil {
		p.error(err)
	}

	return offset
}

func (p *parser) unquoteString(s string) string {
	unquoted, err := strutil.Unquote(s)
	if err != nil {
		p.errorf("error unquoting string %q: %s", s, err)
	}
	return unquoted
}

func parseDuration(ds string) (time.Duration, error) {
	dur, err := model.ParseDuration(ds)
	if err != nil {
		return 0, err
	}
	if dur == 0 {
		return 0, fmt.Errorf("duration must be greater than 0")
	}
	return time.Duration(dur), nil
}
