package htmlreport

// AngularAssemblyViewModel corresponds to the data structure for window.assemblies.
type AngularAssemblyViewModel struct {
	Name    string                  `json:"name"`
	Classes []AngularClassViewModel `json:"classes"`
}

// AngularClassViewModel corresponds to the data structure for classes within window.assemblies.
type AngularClassViewModel struct {
	Name                      string                             `json:"name"`
	ReportPath                string                             `json:"rp"`
	CoveredLines              int                                `json:"cl"`
	UncoveredLines            int                                `json:"ucl"`
	CoverableLines            int                                `json:"cal"`
	TotalLines                int                                `json:"tl"`
	CoveredBranches           int                                `json:"cb"`
	TotalBranches             int                                `json:"tb"`
	CoveredMethods            int                                `json:"cm"`
	FullyCoveredMethods       int                                `json:"fcm"`
	TotalMethods              int                                `json:"tm"`
	LineCoverageHistory       []float64                          `json:"lch,omitempty"`
	BranchCoverageHistory     []float64                          `json:"bch,omitempty"`
	MethodCoverageHistory     []float64                          `json:"mch,omitempty"`
	FullMethodCoverageHistory []float64                          `json:"mfch,omitempty"`
	HistoricCoverages         []AngularHistoricCoverageViewModel `json:"hc"`
	Metrics                   map[string]float64                 `json:"metrics,omitempty"`
}

// AngularHistoricCoverageViewModel corresponds to individual historic coverage data points.
type AngularHistoricCoverageViewModel struct {
	ExecutionTime       string  `json:"et"`
	CoveredLines        int     `json:"cl"`
	CoverableLines      int     `json:"cal"`
	TotalLines          int     `json:"tl"`
	LineCoverageQuota   float64 `json:"lcq"`
	CoveredBranches     int     `json:"cb"`
	TotalBranches       int     `json:"tb"`
	BranchCoverageQuota float64 `json:"bcq"`
}

// AngularMetricViewModel corresponds to the data structure for window.metrics.
type AngularMetricViewModel struct {
	Name           string `json:"name"`
	Abbreviation   string `json:"abbr"`
	ExplanationURL string `json:"explUrl"`
}

// AngularRiskHotspotViewModel corresponds to the data structure for window.riskHotspots.
type AngularRiskHotspotViewModel struct {
	Assembly        string                                    `json:"ass"`
	Class           string                                    `json:"cls"`
	ReportPath      string                                    `json:"rp"`
	MethodName      string                                    `json:"meth"`
	MethodShortName string                                    `json:"methsn"`
	FileIndex       int                                       `json:"fi"`
	Line            int                                       `json:"l"`
	Metrics         []AngularRiskHotspotStatusMetricViewModel `json:"metrics"`
}

// AngularRiskHotspotStatusMetricViewModel represents a single metric's status for a risk hotspot.
type AngularRiskHotspotStatusMetricViewModel struct {
	Value    string `json:"val"` // Value can be string (e.g. "N/A") or number
	Exceeded bool   `json:"ex"`
}

// AngularRiskHotspotMetricHeaderViewModel corresponds to the data structure for window.riskHotspotMetrics (headers).
type AngularRiskHotspotMetricHeaderViewModel struct {
	Name           string `json:"name"`
	Abbreviation   string `json:"abbr"`
	ExplanationURL string `json:"explUrl"`
}
