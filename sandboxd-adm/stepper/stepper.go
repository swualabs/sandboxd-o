package stepper

import (
	"fmt"
	"os"
	"time"

	"sandboxd-o/sandboxd-adm/color"
)

type Stepper struct {
	n int
}

func New() *Stepper {
	return &Stepper{}
}

func (s *Stepper) ts() string {
	return color.Gray(time.Now().Format("15:04:05"))
}

func (s *Stepper) Step(format string, args ...any) {
	s.n++
	fmt.Fprintf(os.Stdout, "%s %s %s\n", s.ts(), color.Cyan(fmt.Sprintf("[%d]", s.n)), color.Bold(fmt.Sprintf(format, args...)))
}

func (s *Stepper) Done(format string, args ...any) {
	fmt.Fprintf(os.Stdout, "%s     %s %s\n", s.ts(), color.Green("✓"), fmt.Sprintf(format, args...))
}

func (s *Stepper) Info(format string, args ...any) {
	fmt.Fprintf(os.Stdout, "%s     %s\n", s.ts(), color.Gray(fmt.Sprintf(format, args...)))
}

func (s *Stepper) Fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s     %s %s\n", s.ts(), color.Red("✗"), color.Red(fmt.Sprintf(format, args...)))
}

func (s *Stepper) Warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s     %s %s\n", s.ts(), color.Yellow("!"), color.Yellow(fmt.Sprintf(format, args...)))
}
