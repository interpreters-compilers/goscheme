package goscheme

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
)

func Eval(exp Expression, env *Env) (ret Expression) {
	for {
		if isNullExp(exp) {
			return NilObj
		}
		if isUndefObj(exp) {
			return undefObj
		}
		if IsNumber(exp) {
			ret = expressionToNumber(exp)
			return
		} else if IsBoolean(exp) {
			return IsTrue(exp)
		} else if IsString(exp) {
			return expToString(exp)
		} else if IsSymbol(exp) {
			var err error
			s, _ := exp.(string)
			ret, err = env.Find(Symbol(s))
			if err != nil {
				panic(err)
			}
			return
		} else if IsSpecialSyntaxExpression(exp, "define") {
			operators, _ := exp.([]Expression)
			ret = evalDefine(operators[1], operators[2:], env)
			return
		} else if IsSpecialSyntaxExpression(exp, "eval") {
			exps, _ := exp.([]Expression)
			return evalEval(exps[1], env)
		} else if IsSpecialSyntaxExpression(exp, "apply") {
			exps, _ := exp.([]Expression)
			return evalApply(exps[1:], env)
		} else if IsSpecialSyntaxExpression(exp, "if") {
			e := exp.([]Expression)
			exp = evalIf(e, env)
		} else if IsSpecialSyntaxExpression(exp, "cond") {
			return evalCond(exp, env)
		} else if IsSpecialSyntaxExpression(exp, "begin") {
			e := exp.([]Expression)
			exp = evalBegin(e, env)
		} else if IsSpecialSyntaxExpression(exp, "lambda") {
			return evalLambda(exp, env)
		} else if IsSpecialSyntaxExpression(exp, "load") {
			exps := exp.([]Expression)
			return evalLoad(exps[1], env)
		} else if IsSpecialSyntaxExpression(exp, "delay") {
			return evalDelay(exp, env)
		} else if IsSpecialSyntaxExpression(exp, "and") {
			return evalAnd(exp, env)
		} else if IsSpecialSyntaxExpression(exp, "or") {
			return evalOr(exp, env)
		} else {
			ops, ok := exp.([]Expression)
			if !ok {
				// exp is just a bottom builtin type, return it directly
				return exp
			}
			if isQuoteExpression(exp) {
				return evalQuote(ops[1], env)
			}
			fn := Eval(ops[0], env)
			switch p := fn.(type) {
			case Function:
				var args []Expression
				for _, arg := range ops[1:] {
					args = append(args, Eval(arg, env))
				}
				return p.Call(args...)
			case *LambdaProcess:
				newEnv := &Env{outer: p.env, frame: make(map[Symbol]Expression)}
				if len(ops[1:]) != len(p.params) {
					panic(fmt.Sprintf("%v\n", p.String()) + "require " + strconv.Itoa(len(p.params)) + " but " + strconv.Itoa(len(ops[1:])) + " provide")
				}
				for i, arg := range ops[1:] {
					newEnv.Set(p.params[i], Eval(arg, env))
				}
				exp = p.Body()
				env = newEnv
			default:
				panic(fmt.Sprintf("%v is not callable", fn))
			}
		}
	}
}
func evalAnd(exp Expression, env *Env) Expression {
	expressions, ok := exp.([]Expression)
	if !ok || len(expressions) < 2 {
		panic("and require at least 1 argument")
	}
	for _, e := range expressions[1:] {
		if !IsTrue(Eval(e, env)) {
			return false
		}
	}
	return true
}

func evalOr(exp Expression, env *Env) Expression {
	expressions, ok := exp.([]Expression)
	if !ok || len(expressions) < 2 {
		panic("or require at least 1 argument")
	}
	for _, e := range expressions[1:] {
		if IsTrue(Eval(e, env)) {
			return true
		}
	}
	return false
}

func evalDelay(exp Expression, env *Env) Expression {
	exps, ok := exp.([]Expression)
	if !ok || len(exps) < 2 {
		panic("delay require one argument")
	}
	return NewThunk(exps[1], env)
}

func isNullExp(exp Expression) bool {
	if exp == nil {
		return true
	}
	switch e := exp.(type) {
	case NilType:
		return true
	case *Pair:
		return e.IsNull()
	case []Expression:
		if len(e) == 0 {
			return true
		}
		return false
	default:
		return false
	}
}

func isLambdaType(expression Expression) bool {
	_, ok := expression.(*LambdaProcess)
	return ok
}

func expToString(exp Expression) String {
	s, _ := exp.(string)
	pattern := regexp.MustCompile(`"((.|[\r\n])*?)"`)
	m := pattern.FindAllStringSubmatch(s, -1)
	return String(m[0][1])
}

func Apply(exp Expression) Expression {
	return nil
}

func evalEval(exp Expression, env *Env) Expression {
	arg := Eval(exp, env)
	if !validEvalExp(arg) {
		panic("error: malformed list")
	}
	expStr := valueToString(arg)
	t := NewTokenizerFromString(expStr)
	tokens := t.Tokens()
	ret, err := Parse(&tokens)
	if err != nil {
		panic(err)
	}
	return EvalAll(ret, env)
}

func validEvalExp(exp Expression) bool {
	switch p := exp.(type) {
	case *Pair:
		if !p.IsList() {
			return false
		}
		return validEvalExp(p.Car) && validEvalExp(p.Cdr)
	default:
		return true
	}
}

func evalApply(exp Expression, env *Env) Expression {
	args, ok := exp.([]Expression)
	if !ok || len(args) != 2 {
		panic("apply require 2 arguments")
	}
	procedure, arg := Eval(args[0], env), Eval(args[1], env)
	if !isList(arg) {
		panic("argument must be a list")
	}
	argList := arg.(*Pair)
	var argSlice = make([]Expression, 0, 1)
	argSlice = append(argSlice, extractList(argList)...)
	var expression []Expression
	expression = append(expression, procedure)
	expression = append(expression, argSlice...)
	return Eval(expression, env)
}

// load other scheme script file
func evalLoad(exp Expression, env *Env) Expression {
	argValue := Eval(exp, env)
	switch v := argValue.(type) {
	case String:
		loadFile(string(v), env)
	case Quote:
		loadFile(string(v), env)
	case *Pair:
		if isList(v) {
			expressions := extractList(v)
			for _, p := range expressions {
				evalLoad(p, env)
			}
		}
	default:
		panic("argument can only contains string, quote or list")
	}
	return undefObj
}

func loadFile(filePath string, env *Env) {
	ext := path.Ext(filePath)
	if ext != ".scm" {
		filePath += ".scm"
	}
	f, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("load %s failed: %s\n", filePath, err)
	}
	i := NewFileInterpreterWithEnv(f, env)
	i.Run()
}

func evalQuote(exp Expression, env *Env) Expression {
	switch v := exp.(type) {
	case Number:
		return v
	case string:
		if IsNumber(v) {
			return expressionToNumber(exp)
		}
		if IsString(exp) {
			return expToString(exp)
		}
		return Quote(v)
	case []Expression:
		var args []Expression
		for _, exp := range v {
			args = append(args, evalQuote(exp, env))
		}
		return listImpl(args...)
	default:
		panic("invalid quote argument")
	}
}

func evalLambda(exp Expression, env *Env) *LambdaProcess {
	se, _ := exp.([]Expression)
	paramOperand := se[1]
	body := se[2:]
	var paramNames []Symbol
	switch p := paramOperand.(type) {
	case []Expression:
		for _, e := range p {
			paramNames = append(paramNames, transExpressionToSymbol(e))
		}
	case Expression:
		paramNames = []Symbol{transExpressionToSymbol(p)}
	}
	return makeLambdaProcess(paramNames, body, env)
}

func isQuoteExpression(exp Expression) bool {
	if exp == "quote" {
		return true
	}
	ops, ok := exp.([]Expression)
	if !ok {
		return false
	}
	return ops[0] == "quote"
}

func evalDefine(s Expression, val []Expression, env *Env) Expression {
	switch se := s.(type) {
	case []Expression:
		var symbols []Symbol
		for _, e := range se {
			symbols = append(symbols, transExpressionToSymbol(e))
		}
		p := makeLambdaProcess(symbols[1:], val, env)
		env.Set(Symbol(symbols[0]), p)
	case Expression:
		if len(val) != 1 {
			panic("define: bad syntax (multiple expressions after identifier")
		}
		env.Set(transExpressionToSymbol(se), Eval(val[0], env))
	}
	return undefObj
}

func transExpressionToSymbol(s Expression) Symbol {
	if IsSymbol(s) {
		s, _ := s.(string)
		return Symbol(s)
	}
	panic(fmt.Sprintf("%v is not a symbol", s))
}

func getParamSymbols(input []string) (ret []Symbol) {
	for _, s := range input {
		ret = append(ret, Symbol(s))
	}
	return
}

func makeLambdaProcess(paramNames []Symbol, body []Expression, env *Env) *LambdaProcess {
	return &LambdaProcess{paramNames, body, env}
}

func EvalAll(exps []Expression, env *Env) (ret Expression) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(r)
		}
	}()
	for _, exp := range exps {
		ret = Eval(exp, env)
	}
	return
}

func expressionToNumber(exp Expression) Number {
	v := exp
	if !IsNumber(v) {
		panic(fmt.Sprintf("%v is not a number", v))
	}
	switch t := v.(type) {
	case Number:
		return t
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return Number(f)
	}
	return 0
}

func conditionOfIfExpression(exp []Expression) Expression {
	return exp[1]
}

func trueExpOfIfExpression(exp []Expression) Expression {
	return exp[2]
}

func elseExpOfIfExpression(exp []Expression) Expression {
	if len(exp) < 4 {
		return undefObj
	}
	return exp[3]
}

func evalIf(exp []Expression, env *Env) Expression {
	if IsTrue(Eval(conditionOfIfExpression(exp), env)) {
		return trueExpOfIfExpression(exp)
	} else {
		return elseExpOfIfExpression(exp)
	}
}

func evalBegin(exp []Expression, env *Env) Expression {
	for _, e := range exp[1 : len(exp)-1] {
		Eval(e, env)
	}
	return exp[len(exp)-1]
}

func evalCond(exp Expression, env *Env) Expression {
	equalIfExp := expandCond(exp)
	return Eval(equalIfExp, env)
}

func makeIf(condition, trueExp, elseExp Expression) []Expression {
	return []Expression{"if", condition, trueExp, elseExp}
}

func condClauses(exp []Expression) []Expression {
	return exp[1:]
}

func expandCond(exp Expression) Expression {
	e := exp.([]Expression)
	return condClausesToIf(condClauses(e))
}

func conditionOfClause(exp []Expression) Expression {
	return exp[0]
}

func processesOfClause(exp []Expression) Expression {
	return exp[1:]
}

func isElseClause(clause Expression) bool {
	switch v := clause.(type) {
	case []Expression:
		return v[0] == "else"
	default:
		return false
	}
}

func condClausesToIf(exp []Expression) Expression {
	if isNullExp(exp) {
		// just a nil obj
		return undefObj
	}
	first := exp[0].([]Expression)
	rest := exp[1:]
	if isElseClause(first) {
		if len(rest) != 0 {
			panic("else clause must in the last position: cond->if")
		}
		return sequenceToExp(processesOfClause(first))
	} else {
		return makeIf(conditionOfClause(first), sequenceToExp(processesOfClause(first)), condClausesToIf(rest))
	}

}

func sequenceToExp(exp Expression) Expression {
	switch exs := exp.(type) {
	case []Expression:
		ret := []Expression{"begin"}
		ret = append(ret, exs...)
		return ret
	case Expression:
		return exs
	}
	return undefObj
}
